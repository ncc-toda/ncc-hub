/**
 * §9.4 動画分割アップロードクライアント。
 *
 * state: idle → initializing → uploading(progress) → completing → done
 *                                  │        │
 *                                  ├─ paused（一時停止）
 *                                  └─ failed / cancelled
 *
 * - チャンクサイズ 20MB 固定、並列3
 * - ネットワークエラー・5xx・429 は指数バックオフ（1s→2s→4s、最大5回）
 * - その他の 4xx は即中止
 * - init 結果を localStorage（works.uploads）に保存し、リロード後の再開に使う
 */

import {
  ApiError,
  getWork,
  videoCancel,
  videoChunk,
  videoChunkStatus,
  videoComplete,
  videoInit,
  type WorkRecord,
} from './api';
import { removeUpload, setUpload, type StoredUpload } from './keys';

export const CHUNK_SIZE = 20_971_520; // 20MB 固定（サーバーはこれ以外を 400 で拒否）
export const VIDEO_EXTS = ['mp4', 'mov', 'm4v', 'webm', 'mkv', 'avi'];
const PARALLEL = 3;
const MAX_RETRIES = 5;
const MAX_COMPLETE_ATTEMPTS = 3;

export type UploadPhase =
  | 'idle'
  | 'initializing'
  | 'uploading'
  | 'paused'
  | 'completing'
  | 'done'
  | 'failed'
  | 'cancelled';

export interface UploadSnapshot {
  phase: UploadPhase;
  sentBytes: number;
  totalBytes: number;
  error: string;
}

export function fileExt(name: string): string {
  const m = /\.([A-Za-z0-9]+)$/.exec(name);
  return m ? m[1].toLowerCase() : '';
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export interface VideoUploaderOptions {
  slug: string;
  workId: string;
  editKey: string;
  file: File;
  onChange?: (snapshot: UploadSnapshot) => void;
}

export class VideoUploader {
  private readonly slug: string;
  private readonly workId: string;
  private readonly editKey: string;
  private readonly file: File;
  private readonly onChange: (snapshot: UploadSnapshot) => void;

  private uploadId = '';
  private totalChunks = 0;
  private pending: number[] = [];
  private received = new Set<number>();
  private paused = false;
  private cancelled = false;
  private stopped = false;
  private fatal: unknown = null;
  private controllers = new Set<AbortController>();
  private wakeups: Array<() => void> = [];

  phase: UploadPhase = 'idle';
  errorMessage = '';

  constructor(opts: VideoUploaderOptions) {
    this.slug = opts.slug;
    this.workId = opts.workId;
    this.editKey = opts.editKey;
    this.file = opts.file;
    this.onChange = opts.onChange ?? (() => undefined);
  }

  /**
   * アップロードを実行する。
   * - `resumeFrom` を渡すと init を省略し、受信済みチャンクを問い合わせて残りだけ送る
   * - 完了時は complete 後の works レコードを返す
   * - キャンセル時は null を返す（cancel() 側でサーバー掃除・localStorage 削除済み）
   * - 失敗時は ApiError / Error を投げる（phase は 'failed'）
   */
  async start(resumeFrom?: StoredUpload): Promise<WorkRecord | null> {
    try {
      this.setPhase('initializing');

      if (resumeFrom) {
        this.uploadId = resumeFrom.uploadId;
        this.totalChunks = resumeFrom.totalChunks;
        const st = await videoChunkStatus(this.slug, this.workId, this.editKey, this.uploadId);
        if (st.size !== this.file.size) {
          throw new ApiError(0, '選択されたファイルが前回と一致しません', 'resume_mismatch');
        }
        this.totalChunks = st.total_chunks;
        this.received = new Set(st.received_indexes ?? []);
      } else {
        const r = await videoInit(this.slug, this.workId, this.editKey, {
          filename: this.file.name,
          size: this.file.size,
          chunk_size: CHUNK_SIZE,
        });
        this.uploadId = r.upload_id;
        this.totalChunks = r.total_chunks;
        this.received = new Set();
        setUpload(this.workId, {
          uploadId: this.uploadId,
          size: this.file.size,
          ext: fileExt(this.file.name),
          totalChunks: this.totalChunks,
        });
      }

      this.pending = [];
      for (let i = 0; i < this.totalChunks; i++) {
        if (!this.received.has(i)) this.pending.push(i);
      }

      this.setPhase('uploading');
      await this.drain();
      if (this.cancelled) return null;
      if (this.fatal) throw this.fatal;

      this.setPhase('completing');
      const work = await this.completeWithRetry();
      if (this.cancelled) return null;

      removeUpload(this.workId);
      this.setPhase('done');
      return work;
    } catch (err) {
      if (this.cancelled) return null;
      this.stopped = true;
      this.abortAll();
      this.wakeAll();
      // 404 = アップロードがサーバー側で消えている（掃除済み等）→ 再開情報は無効
      if (err instanceof ApiError && err.status === 404) removeUpload(this.workId);
      this.errorMessage =
        err instanceof Error && err.message ? err.message : 'アップロードに失敗しました';
      this.setPhase('failed');
      throw err;
    }
  }

  pause(): void {
    if (this.phase !== 'uploading') return;
    this.paused = true;
    this.abortAll();
    this.setPhase('paused');
  }

  resume(): void {
    if (!this.paused || this.cancelled || this.stopped) return;
    this.paused = false;
    this.setPhase('uploading');
    this.wakeAll();
  }

  /** 中止。サーバー側の一時領域も削除する。 */
  async cancel(): Promise<void> {
    if (this.cancelled) return;
    this.cancelled = true;
    this.stopped = true;
    this.abortAll();
    this.wakeAll();
    if (this.uploadId) {
      try {
        await videoCancel(this.slug, this.workId, this.editKey, this.uploadId);
      } catch {
        // 掃除 cron に任せる
      }
    }
    removeUpload(this.workId);
    this.errorMessage = '';
    this.setPhase('cancelled');
  }

  // ---- 内部 -------------------------------------------------------------

  private setPhase(phase: UploadPhase): void {
    this.phase = phase;
    this.emit();
  }

  private emit(): void {
    this.onChange({
      phase: this.phase,
      sentBytes: this.sentBytes(),
      totalBytes: this.file.size,
      error: this.errorMessage,
    });
  }

  private chunkBytes(index: number): number {
    if (index === this.totalChunks - 1) {
      return this.file.size - CHUNK_SIZE * (this.totalChunks - 1);
    }
    return CHUNK_SIZE;
  }

  private sentBytes(): number {
    if (this.totalChunks === 0) return 0;
    let bytes = 0;
    for (const i of this.received) bytes += this.chunkBytes(i);
    return Math.min(bytes, this.file.size);
  }

  private abortAll(): void {
    for (const c of this.controllers) c.abort();
    this.controllers.clear();
  }

  private waitWake(): Promise<void> {
    return new Promise((resolve) => this.wakeups.push(resolve));
  }

  private wakeAll(): void {
    const waiters = this.wakeups;
    this.wakeups = [];
    for (const w of waiters) w();
  }

  private async drain(): Promise<void> {
    const workers = Math.max(1, Math.min(PARALLEL, this.pending.length));
    await Promise.all(Array.from({ length: workers }, () => this.worker()));
  }

  private async worker(): Promise<void> {
    for (;;) {
      if (this.stopped) return;
      if (this.paused) {
        await this.waitWake();
        continue;
      }
      const index = this.pending.shift();
      if (index === undefined) return;
      try {
        await this.sendChunk(index);
      } catch (err) {
        if (!this.fatal) this.fatal = err;
        this.stopped = true;
        this.abortAll();
        this.wakeAll();
        return;
      }
    }
  }

  private async sendChunk(index: number): Promise<void> {
    const start = index * CHUNK_SIZE;
    const blob = this.file.slice(start, Math.min(start + CHUNK_SIZE, this.file.size));
    let attempt = 0;
    for (;;) {
      if (this.stopped) return;
      if (this.paused) {
        this.pending.unshift(index);
        return;
      }
      const ctrl = new AbortController();
      this.controllers.add(ctrl);
      try {
        await videoChunk(
          this.slug,
          this.workId,
          this.editKey,
          this.uploadId,
          index,
          blob,
          ctrl.signal,
        );
        this.received.add(index);
        this.emit();
        return;
      } catch (err) {
        // 一時停止・中止による abort は再キューして静かに戻る
        if (this.paused || this.stopped) {
          this.pending.unshift(index);
          return;
        }
        const retriable =
          err instanceof ApiError
            ? err.status === 0 || err.status === 429 || err.status >= 500
            : true;
        if (!retriable) throw err;
        attempt += 1;
        if (attempt > MAX_RETRIES) throw err;
        await sleep(Math.min(1000 * 2 ** (attempt - 1), 4000));
      } finally {
        this.controllers.delete(ctrl);
      }
    }
  }

  private async completeWithRetry(): Promise<WorkRecord> {
    for (let attempt = 0; attempt < MAX_COMPLETE_ATTEMPTS; attempt++) {
      try {
        const r = await videoComplete(this.slug, this.workId, this.editKey, this.uploadId);
        return r.work;
      } catch (err) {
        const missing =
          err instanceof ApiError && err.status === 409 && Array.isArray(err.data['missing'])
            ? (err.data['missing'] as number[])
            : null;
        if (!missing || attempt === MAX_COMPLETE_ATTEMPTS - 1) throw err;
        for (const m of missing) this.received.delete(m);
        this.pending = missing.slice();
        this.setPhase('uploading');
        await this.drain();
        if (this.cancelled) throw new Error('cancelled');
        if (this.fatal) throw this.fatal;
        this.setPhase('completing');
      }
    }
    throw new ApiError(
      409,
      'アップロードを完了できませんでした。時間をおいてもう一度お試しください',
      'upload_state_conflict',
    );
  }
}

/**
 * complete 後の変換待ちポーリング。5秒間隔・最大30分。
 * video_status が processing/uploading 以外になったらそのレコードを返す。
 * タイムアウト時は Error を投げる。`shouldStop()` が true を返すと 'stopped' Error で抜ける。
 */
export async function pollVideoStatus(
  slug: string,
  workId: string,
  opts: {
    intervalMs?: number;
    timeoutMs?: number;
    onUpdate?: (work: WorkRecord) => void;
    shouldStop?: () => boolean;
  } = {},
): Promise<WorkRecord> {
  const interval = opts.intervalMs ?? 5000;
  const deadline = Date.now() + (opts.timeoutMs ?? 30 * 60 * 1000);
  for (;;) {
    if (opts.shouldStop?.()) throw new Error('stopped');
    let work: WorkRecord | null = null;
    try {
      work = await getWork(slug, workId);
    } catch {
      // 一時的な失敗は次の周回で再試行
    }
    if (work) {
      opts.onUpdate?.(work);
      if (work.video_status !== 'processing' && work.video_status !== 'uploading') return work;
    }
    if (Date.now() > deadline) {
      throw new Error('変換に時間がかかっています。しばらくしてから作品ページを確認してください');
    }
    await sleep(interval);
  }
}
