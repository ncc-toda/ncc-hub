/**
 * 最小限の history API ルーター。
 * `navigate()` は pushState/replaceState 後に popstate イベントを発火させ、
 * App.svelte がそれを購読して再描画する。
 */

export type RouteName =
  | 'home'
  | 'keys'
  | 'list'
  | 'new'
  | 'detail'
  | 'edit'
  | 'notfound';

export interface Route {
  name: RouteName;
  params: Record<string, string>;
}

export function matchRoute(pathname: string): Route {
  const parts = pathname.split('/').filter(Boolean);
  if (parts.length === 0) return { name: 'home', params: {} };
  if (parts.length === 1 && parts[0] === 'keys') return { name: 'keys', params: {} };
  if (parts[0] === 'e' && parts.length >= 2) {
    const slug = decodeURIComponent(parts[1]);
    if (parts.length === 2) return { name: 'list', params: { slug } };
    if (parts.length === 3 && parts[2] === 'new') return { name: 'new', params: { slug } };
    if (parts.length === 4 && parts[2] === 'w') {
      return { name: 'detail', params: { slug, id: decodeURIComponent(parts[3]) } };
    }
    if (parts.length === 5 && parts[2] === 'w' && parts[4] === 'edit') {
      return { name: 'edit', params: { slug, id: decodeURIComponent(parts[3]) } };
    }
  }
  return { name: 'notfound', params: {} };
}

export function navigate(path: string, opts: { replace?: boolean } = {}): void {
  if (opts.replace) {
    history.replaceState(null, '', path);
  } else {
    history.pushState(null, '', path);
  }
  window.dispatchEvent(new PopStateEvent('popstate'));
}

/** `<a href="..." onclick={onLinkClick}>` 用。修飾キー付きクリックはブラウザに任せる。 */
export function onLinkClick(e: MouseEvent): void {
  if (e.defaultPrevented || e.button !== 0) return;
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const a = e.currentTarget as HTMLAnchorElement | null;
  const href = a?.getAttribute('href');
  if (!href || !href.startsWith('/')) return;
  e.preventDefault();
  navigate(href);
}
