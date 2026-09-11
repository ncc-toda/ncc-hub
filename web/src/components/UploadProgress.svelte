<script lang="ts">
  import type { UploadSnapshot } from '../lib/upload';

  let {
    snapshot,
    onPause,
    onResume,
    onCancel,
    onRetry,
  }: {
    snapshot: UploadSnapshot;
    onPause: () => void;
    onResume: () => void;
    onCancel: () => void;
    onRetry?: () => void;
  } = $props();

  const percent = $derived(
    snapshot.totalBytes > 0 ? Math.floor((snapshot.sentBytes / snapshot.totalBytes) * 100) : 0,
  );

  function fmtBytes(n: number): string {
    if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(2)} GB`;
    if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(1)} MB`;
    if (n >= 1024) return `${(n / 1024).toFixed(0)} KB`;
    return `${n} B`;
  }

  const phaseLabel = $derived.by(() => {
    switch (snapshot.phase) {
      case 'initializing':
        return '準備中…';
      case 'uploading':
        return 'アップロード中…';
      case 'paused':
        return '一時停止中';
      case 'completing':
        return 'ファイルを結合中…';
      case 'done':
        return 'アップロード完了';
      case 'failed':
        return 'アップロードに失敗しました';
      case 'cancelled':
        return '中止しました';
      default:
        return '';
    }
  });
</script>

<div class="upload-box card">
  <div class="row">
    <strong>{phaseLabel}</strong>
    <span class="pct">{percent}%</span>
  </div>
  <div class="bar" role="progressbar" aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100}>
    <div class="fill" style={`width:${percent}%`}></div>
  </div>
  <p class="bytes">{fmtBytes(snapshot.sentBytes)} / {fmtBytes(snapshot.totalBytes)}</p>

  {#if snapshot.phase === 'failed' && snapshot.error}
    <p class="error-text">{snapshot.error}</p>
  {/if}

  <div class="actions">
    {#if snapshot.phase === 'uploading'}
      <button type="button" class="btn btn-sm" onclick={onPause}>一時停止</button>
      <button type="button" class="btn btn-sm btn-danger" onclick={onCancel}>中止</button>
    {:else if snapshot.phase === 'paused'}
      <button type="button" class="btn btn-sm btn-primary" onclick={onResume}>再開</button>
      <button type="button" class="btn btn-sm btn-danger" onclick={onCancel}>中止</button>
    {:else if snapshot.phase === 'failed' && onRetry}
      <button type="button" class="btn btn-sm btn-primary" onclick={onRetry}>もう一度試す</button>
      <button type="button" class="btn btn-sm btn-danger" onclick={onCancel}>中止</button>
    {/if}
  </div>

  {#if snapshot.phase === 'uploading' || snapshot.phase === 'paused' || snapshot.phase === 'completing'}
    <p class="hint">アップロードが終わるまでこのページを閉じないでください。</p>
  {/if}
</div>

<style>
  .upload-box {
    padding: 16px;
    margin: 16px 0;
  }

  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .pct {
    font-variant-numeric: tabular-nums;
    font-weight: 700;
  }

  .bar {
    height: 14px;
    border: var(--hairline);
    border-radius: 3px;
    background: var(--surface-2);
    overflow: hidden;
  }

  .fill {
    height: 100%;
    background: var(--accent);
    transition: width 0.3s ease;
  }

  .bytes {
    color: var(--muted);
    font-size: 0.85rem;
    margin: 6px 0 0;
    font-variant-numeric: tabular-nums;
  }

  .actions {
    display: flex;
    gap: 8px;
    margin-top: 12px;
  }
</style>
