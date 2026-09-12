import { ApiError } from './api';

/** 自前コードが投げる「そのままユーザーに見せてよい日本語メッセージ」のマーカー */
export class AppError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'AppError';
  }
}

/**
 * ユーザー向けエラーメッセージへの変換。
 * ApiError / AppError 以外の message（JS 例外文など）は UI に出さない。
 */
export function errMsg(
  err: unknown,
  fallback = 'エラーが発生しました。時間をおいてもう一度お試しください',
): string {
  if (err instanceof ApiError || err instanceof AppError) return err.message;
  console.error(err);
  return fallback;
}
