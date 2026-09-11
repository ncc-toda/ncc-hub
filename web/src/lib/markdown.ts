/**
 * Markdown → 安全な HTML。
 * - 生HTMLは禁止（DOMPurify が全て除去）
 * - 画像記法は禁止（<img> を許可しない）
 * - リンクは http(s) のみ。rel="noopener nofollow" target="_blank" を強制
 */

import { marked } from 'marked';
import DOMPurify from 'dompurify';

marked.setOptions({ gfm: true, breaks: true });

const ALLOWED_TAGS = [
  'p',
  'br',
  'strong',
  'b',
  'em',
  'i',
  'del',
  's',
  'code',
  'pre',
  'blockquote',
  'ul',
  'ol',
  'li',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'a',
  'hr',
  'table',
  'thead',
  'tbody',
  'tr',
  'th',
  'td',
];

const ALLOWED_ATTR = ['href'];

DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName !== 'A') return;
  const href = node.getAttribute('href') ?? '';
  if (!/^https?:\/\//i.test(href)) {
    node.removeAttribute('href');
    return;
  }
  node.setAttribute('target', '_blank');
  node.setAttribute('rel', 'noopener nofollow');
});

export function renderMarkdown(src: string): string {
  if (!src) return '';
  const html = marked.parse(src, { async: false });
  return DOMPurify.sanitize(html, { ALLOWED_TAGS, ALLOWED_ATTR });
}
