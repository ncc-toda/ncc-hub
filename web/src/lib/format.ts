/** PocketBase の "YYYY-MM-DD HH:MM:SS.sssZ"（スペース区切り）も受けて YYYY.MM.DD で返す */
export function formatDate(s: string): string {
  const d = new Date(s.replace(' ', 'T'));
  if (Number.isNaN(d.getTime())) return s;
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}.${m}.${day}`;
}

export function formatBytes(n: number): string {
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(2)} GB`;
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${n} B`;
}

export function formatGB(bytes: number): string {
  return `${(bytes / 1024 ** 3).toFixed(1)}GB`;
}
