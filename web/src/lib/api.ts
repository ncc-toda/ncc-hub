/**
 * API クライアント。
 * - 読み取りは PocketBase 標準 Records API（X-Event-Key 必須）
 * - 書き込みは /api/x/ のカスタムAPI
 * - エラーは ApiError {status, message, code} に正規化する
 */

import { getDeviceId, getEventKey } from './keys';

export type VideoStatus = 'none' | 'uploading' | 'processing' | 'ready' | 'failed';

export interface WorkLink {
  title: string;
  url: string;
}

export interface EventRecord {
  id: string;
  collectionId: string;
  collectionName: string;
  name: string;
  slug: string;
  description: string;
  submissions_open: boolean;
  max_video_bytes: number;
  created: string;
  updated: string;
}

export interface WorkRecord {
  id: string;
  collectionId: string;
  collectionName: string;
  event: string;
  description: string;
  images: string[];
  video: string;
  video_status: VideoStatus;
  video_error: string;
  thumbnail: string;
  video_url: string;
  demo_url: string;
  github_url: string;
  tags: string[];
  links: WorkLink[];
  like_count: number;
  created: string;
  updated: string;
}

export class ApiError extends Error {
  status: number;
  code: string;
  data: Record<string, unknown>;
  /** 422 validation の項目別メッセージ（{field: message}） */
  fields: Record<string, string>;

  constructor(status: number, message: string, code = 'unknown', data: Record<string, unknown> = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.data = data;
    this.fields = {};
    const f = data['fields'];
    if (f && typeof f === 'object') {
      for (const [k, v] of Object.entries(f as Record<string, unknown>)) {
        if (typeof v === 'string') {
          this.fields[k] = v;
        } else if (v && typeof v === 'object' && typeof (v as { message?: unknown }).message === 'string') {
          this.fields[k] = (v as { message: string }).message;
        }
      }
    }
  }
}

const STATUS_MESSAGES: Record<number, string> = {
  0: 'ネットワークエラーです。通信環境を確認してもう一度お試しください',
  400: 'リクエストが正しくありません',
  401: '合言葉が必要です',
  403: '権限がありません',
  404: '見つかりませんでした',
  409: 'アップロードの状態が一致しません',
  413: 'サイズが大きすぎます',
  422: '入力内容を確認してください',
  423: '受付は終了しました',
  429: 'アクセスが集中しています。しばらく待ってからもう一度お試しください',
  500: 'サーバーでエラーが発生しました',
  507: 'サーバーの空き容量が足りません。先生に連絡してください',
};

export interface RequestOpts {
  /** 指定すると localStorage の合言葉を X-Event-Key に付与 */
  slug?: string;
  /** 指定すると X-Edit-Key に付与 */
  editKey?: string;
  /** true なら X-Device-Id を付与 */
  deviceId?: boolean;
  signal?: AbortSignal;
}

export async function apiFetch<T>(path: string, init: RequestInit = {}, opts: RequestOpts = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (opts.slug) {
    const key = getEventKey(opts.slug);
    if (key) headers.set('X-Event-Key', key);
  }
  if (opts.editKey) headers.set('X-Edit-Key', opts.editKey);
  if (opts.deviceId) headers.set('X-Device-Id', getDeviceId());

  let res: Response;
  try {
    res = await fetch(path, { ...init, headers, signal: opts.signal ?? init.signal ?? null });
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err;
    throw new ApiError(0, STATUS_MESSAGES[0], 'network');
  }

  if (res.status === 204) return undefined as T;

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }

  if (!res.ok) {
    const b = (body ?? {}) as { message?: unknown; data?: unknown };
    const data = b.data && typeof b.data === 'object' ? (b.data as Record<string, unknown>) : {};
    const code = typeof data['code'] === 'string' ? (data['code'] as string) : 'unknown';
    const message =
      typeof b.message === 'string' && b.message
        ? b.message
        : (STATUS_MESSAGES[res.status] ?? `エラーが発生しました（${res.status}）`);
    throw new ApiError(res.status, message, code, data);
  }

  return body as T;
}

function normalizeWork(w: WorkRecord): WorkRecord {
  return {
    ...w,
    images: Array.isArray(w.images) ? w.images : [],
    tags: Array.isArray(w.tags) ? w.tags : [],
    links: Array.isArray(w.links) ? w.links : [],
    github_url: typeof w.github_url === 'string' ? w.github_url : '',
    like_count: typeof w.like_count === 'number' ? w.like_count : 0,
  };
}

/** create / video complete は `{ work }`。PATCH は過去にレコード直返しだった。 */
function workFromBody(body: unknown): WorkRecord {
  if (!body || typeof body !== 'object') {
    throw new ApiError(500, STATUS_MESSAGES[500], 'unknown');
  }
  const rec = body as Record<string, unknown>;
  const inner = rec.work;
  if (inner && typeof inner === 'object' && !Array.isArray(inner)) {
    return normalizeWork(inner as WorkRecord);
  }
  if (typeof rec.id === 'string') {
    return normalizeWork(body as WorkRecord);
  }
  throw new ApiError(500, STATUS_MESSAGES[500], 'unknown');
}

// ---- 読み取り（標準 Records API） ---------------------------------------

/**
 * slug からイベントを解決する。
 * 合言葉が間違っている／ルールに弾かれた場合、PocketBase は 400 系を返すか
 * 空の items を返すことがあるため、「空 or エラー」はどちらも合言葉ゲート行きに正規化する。
 */
export async function resolveEvent(slug: string): Promise<EventRecord> {
  const filter = encodeURIComponent(`(slug='${slug.replace(/'/g, '')}')`);
  const data = await apiFetch<{ items?: EventRecord[] }>(
    `/api/collections/events/records?filter=${filter}&perPage=1`,
    {},
    { slug },
  );
  const ev = data.items?.[0];
  if (!ev) throw new ApiError(401, '合言葉が違います', 'event_key_invalid');
  return ev;
}

export async function listWorks(slug: string, eventId: string): Promise<WorkRecord[]> {
  const filter = encodeURIComponent(`(event='${eventId}')`);
  const data = await apiFetch<{ items?: WorkRecord[] }>(
    `/api/collections/works/records?filter=${filter}&sort=-created&perPage=500`,
    {},
    { slug },
  );
  return (data.items ?? []).map(normalizeWork);
}

export async function getWork(slug: string, id: string): Promise<WorkRecord> {
  const w = await apiFetch<WorkRecord>(
    `/api/collections/works/records/${encodeURIComponent(id)}`,
    {},
    { slug },
  );
  return normalizeWork(w);
}

/** ファイル配信URL。thumb=true で 600x400 サムネイル変種。 */
export function fileUrl(
  record: { collectionId: string; id: string },
  filename: string,
  thumb = false,
): string {
  const base = `/api/files/${record.collectionId}/${record.id}/${encodeURIComponent(filename)}`;
  return thumb ? `${base}?thumb=600x400` : base;
}

// ---- 書き込み（カスタムAPI /api/x/） ------------------------------------

/** 編集キーと作品コードが返るのはこの呼び出しだけ（SPEC §8.2 手順7）。 */
export async function createWork(
  slug: string,
  form: FormData,
): Promise<{ work: WorkRecord; edit_key: string; work_code: string }> {
  const r = await apiFetch<{ work: WorkRecord; edit_key: string; work_code: string }>(
    '/api/x/works',
    { method: 'POST', body: form },
    { slug },
  );
  return { work: normalizeWork(r.work), edit_key: r.edit_key, work_code: r.work_code };
}

export async function updateWork(
  slug: string,
  id: string,
  editKey: string,
  form: FormData,
): Promise<{ work: WorkRecord }> {
  const r = await apiFetch<unknown>(
    `/api/x/works/${encodeURIComponent(id)}`,
    { method: 'PATCH', body: form },
    { slug, editKey },
  );
  return { work: workFromBody(r) };
}

export function deleteWork(slug: string, id: string, editKey: string): Promise<void> {
  return apiFetch<void>(`/api/x/works/${encodeURIComponent(id)}`, { method: 'DELETE' }, { slug, editKey });
}

// ---- いいね -------------------------------------------------------------

export function toggleLike(slug: string, workId: string): Promise<{ liked: boolean; like_count: number }> {
  return apiFetch<{ liked: boolean; like_count: number }>(
    `/api/x/works/${encodeURIComponent(workId)}/like`,
    { method: 'POST' },
    { slug, deviceId: true },
  );
}

export async function getLikedWorkIds(slug: string, eventId: string): Promise<string[]> {
  const r = await apiFetch<{ work_ids?: string[] }>(
    `/api/x/likes?event=${encodeURIComponent(eventId)}`,
    {},
    { slug, deviceId: true },
  );
  return r.work_ids ?? [];
}

// ---- 動画分割アップロード -----------------------------------------------

export function videoInit(
  slug: string,
  workId: string,
  editKey: string,
  payload: { filename: string; size: number; chunk_size: number },
): Promise<{ upload_id: string; chunk_size: number; total_chunks: number }> {
  return apiFetch(
    `/api/x/works/${encodeURIComponent(workId)}/video/init`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    },
    { slug, editKey },
  );
}

export function videoChunk(
  slug: string,
  workId: string,
  editKey: string,
  uploadId: string,
  index: number,
  chunk: Blob,
  signal?: AbortSignal,
): Promise<{ index: number; received: number }> {
  return apiFetch(
    `/api/x/works/${encodeURIComponent(workId)}/video/chunk/${encodeURIComponent(uploadId)}/${index}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: chunk,
    },
    { slug, editKey, signal },
  );
}

export function videoChunkStatus(
  slug: string,
  workId: string,
  editKey: string,
  uploadId: string,
): Promise<{ received_indexes: number[]; total_chunks: number; size: number }> {
  return apiFetch(
    `/api/x/works/${encodeURIComponent(workId)}/video/chunk/${encodeURIComponent(uploadId)}`,
    {},
    { slug, editKey },
  );
}

export async function videoComplete(
  slug: string,
  workId: string,
  editKey: string,
  uploadId: string,
): Promise<{ work: WorkRecord }> {
  const r = await apiFetch<unknown>(
    `/api/x/works/${encodeURIComponent(workId)}/video/complete/${encodeURIComponent(uploadId)}`,
    { method: 'POST' },
    { slug, editKey },
  );
  return { work: workFromBody(r) };
}

export function videoCancel(slug: string, workId: string, editKey: string, uploadId: string): Promise<void> {
  return apiFetch<void>(
    `/api/x/works/${encodeURIComponent(workId)}/video/chunk/${encodeURIComponent(uploadId)}`,
    { method: 'DELETE' },
    { slug, editKey },
  );
}
