/**
 * §9.5 端末ID・鍵の保管（localStorage）
 *
 *   works.deviceId   … UUID v4（初回生成）
 *   works.eventKeys  … { [slug]: passphrase }
 *   works.editKeys   … { [workId]: editKey }
 *   works.uploads    … { [workId]: { uploadId, size, ext, totalChunks } }
 */

const DEVICE_ID_KEY = 'works.deviceId';
const EVENT_KEYS_KEY = 'works.eventKeys';
const EDIT_KEYS_KEY = 'works.editKeys';
const UPLOADS_KEY = 'works.uploads';

export interface StoredUpload {
  uploadId: string;
  size: number;
  ext: string;
  totalChunks: number;
}

function readJson<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return fallback;
    const parsed = JSON.parse(raw) as unknown;
    if (parsed && typeof parsed === 'object') return parsed as T;
    return fallback;
  } catch {
    return fallback;
  }
}

function writeJson(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // ストレージ不可（プライベートモード等）は黙って諦める
  }
}

// ---- 端末ID -------------------------------------------------------------

export function getDeviceId(): string {
  let id = localStorage.getItem(DEVICE_ID_KEY);
  if (!id || !/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(id)) {
    id = crypto.randomUUID();
    try {
      localStorage.setItem(DEVICE_ID_KEY, id);
    } catch {
      // 保存できなくてもセッション中は同じ値を使えるよう続行
    }
  }
  return id;
}

// ---- 合言葉（イベントキー） ---------------------------------------------

export function getEventKeys(): Record<string, string> {
  return readJson<Record<string, string>>(EVENT_KEYS_KEY, {});
}

export function getEventKey(slug: string): string | null {
  return getEventKeys()[slug] ?? null;
}

/** 再設定時は末尾に移動する（= 最後に使ったイベントが末尾になる）。 */
export function setEventKey(slug: string, passphrase: string): void {
  const keys = getEventKeys();
  delete keys[slug];
  keys[slug] = passphrase;
  writeJson(EVENT_KEYS_KEY, keys);
}

export function removeEventKey(slug: string): void {
  const keys = getEventKeys();
  delete keys[slug];
  writeJson(EVENT_KEYS_KEY, keys);
}

// ---- 編集キー -----------------------------------------------------------

export function getEditKeys(): Record<string, string> {
  return readJson<Record<string, string>>(EDIT_KEYS_KEY, {});
}

export function getEditKey(workId: string): string | null {
  return getEditKeys()[workId] ?? null;
}

export function setEditKey(workId: string, editKey: string): void {
  const keys = getEditKeys();
  keys[workId] = editKey;
  writeJson(EDIT_KEYS_KEY, keys);
}

export function removeEditKey(workId: string): void {
  const keys = getEditKeys();
  delete keys[workId];
  writeJson(EDIT_KEYS_KEY, keys);
}

// ---- 分割アップロードの再開情報 -----------------------------------------

export function getUploads(): Record<string, StoredUpload> {
  return readJson<Record<string, StoredUpload>>(UPLOADS_KEY, {});
}

export function getUpload(workId: string): StoredUpload | null {
  const u = getUploads()[workId];
  if (!u || typeof u.uploadId !== 'string' || typeof u.size !== 'number') return null;
  return u;
}

export function setUpload(workId: string, meta: StoredUpload): void {
  const uploads = getUploads();
  uploads[workId] = meta;
  writeJson(UPLOADS_KEY, uploads);
}

export function removeUpload(workId: string): void {
  const uploads = getUploads();
  delete uploads[workId];
  writeJson(UPLOADS_KEY, uploads);
}
