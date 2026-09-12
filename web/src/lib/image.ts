/**
 * §9.2 画像のクライアント側処理。
 * - createImageBitmap → canvas → toBlob で再エンコード（EXIF・GPS が落ちる）
 * - 長辺 2048px に縮小
 * - 透過あり（アルファチャンネル）は PNG、それ以外は JPEG 0.85
 * - GIF はアニメーションを壊すので再エンコードせずそのまま（名前だけ固定名に）
 * - 送信ファイル名は固定名（サーバー側でさらに乱数名になる）
 */

import { AppError } from './errors';

const MAX_LONG_EDGE = 2048;
const JPEG_QUALITY = 0.85;

export const IMAGE_ACCEPT = 'image/jpeg,image/png,image/webp,image/gif';

function hasAlpha(ctx: CanvasRenderingContext2D, width: number, height: number): boolean {
  const data = ctx.getImageData(0, 0, width, height).data;
  for (let i = 3; i < data.length; i += 4) {
    if (data[i] < 255) return true;
  }
  return false;
}

/**
 * 1枚の画像を送信用に処理する。失敗時は日本語メッセージの Error を投げる。
 * `seq` はフォーム内での連番（固定ファイル名の重複回避用）。
 */
export async function processImage(file: File, seq = 0): Promise<File> {
  if (file.type === 'image/gif') {
    return new File([file], `image_${seq}.gif`, { type: 'image/gif' });
  }

  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    throw new AppError('画像を読み込めませんでした。別の画像をお試しください');
  }

  try {
    const scale = Math.min(1, MAX_LONG_EDGE / Math.max(bitmap.width, bitmap.height));
    const width = Math.max(1, Math.round(bitmap.width * scale));
    const height = Math.max(1, Math.round(bitmap.height * scale));

    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new AppError('画像を処理できませんでした');
    ctx.drawImage(bitmap, 0, 0, width, height);

    // JPEG 由来は透過を持たないので走査を省略
    const alpha =
      (file.type === 'image/png' || file.type === 'image/webp') && hasAlpha(ctx, width, height);
    const type = alpha ? 'image/png' : 'image/jpeg';
    const ext = alpha ? 'png' : 'jpg';

    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, type, alpha ? undefined : JPEG_QUALITY),
    );
    if (!blob) throw new AppError('画像を変換できませんでした');
    return new File([blob], `image_${seq}.${ext}`, { type });
  } finally {
    bitmap.close();
  }
}
