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

/** 記法だけを落とす。行単位で呼ぶ前提（SPEC §9.1 の見出し生成用）。 */
function stripMarkdown(line: string): string {
  const s = line
    .replace(/^\s{0,3}(#{1,6}\s+|>\s?|[-*+]\s+|\d+[.)]\s+)/, '') // 見出し・引用・リスト
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1') // 画像
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1') // リンク
    .replace(/(\*\*|__|~~|\*|_|`)/g, '') // 強調・コード
    .replace(/<[^>]*>/g, '') // 生HTML
    .replace(/\s+/g, ' ')
    .trim();
  // 水平線や表の区切り行だけになったものは見出しにしない
  return /^[-*_=|:\s]+$/.test(s) ? '' : s;
}

/**
 * 説明文から見出し用のプレーンテキストを作る（SPEC §9.1）。
 * タイトル欄が無いので、最初の意味のある行を最大 max 文字で見出しに使う。
 */
export function plainExcerpt(src: string, max = 40): string {
  for (const line of (src ?? '').split('\n')) {
    const text = stripMarkdown(line);
    if (text) return text.length > max ? text.slice(0, max) + '…' : text;
  }
  return '';
}

export function renderMarkdown(src: string): string {
  if (!src) return '';
  const html = marked.parse(src, { async: false });
  return DOMPurify.sanitize(html, { ALLOWED_TAGS, ALLOWED_ATTR });
}
