import { getEventKey, setEventKey } from './keys';

/** Vite の開発サーバーでのみ true。vite build 後は消える。 */
export const isDevFlavor = import.meta.env.DEV;

/** server/internal/devseed と同じ値。SPEC §5.5 */
export const DEV_EVENT_SLUG = 'dev';
export const DEV_PASSPHRASE = 'dev-aikotoba';

/** 未保存のときだけ開発用合言葉を足す。上書きはしない。 */
export function ensureDevEventKey(): void {
  if (!isDevFlavor) return;
  if (getEventKey(DEV_EVENT_SLUG)) return;
  setEventKey(DEV_EVENT_SLUG, DEV_PASSPHRASE);
}
