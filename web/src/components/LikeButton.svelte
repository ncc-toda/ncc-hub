<script lang="ts">
  import { ApiError, toggleLike } from '../lib/api';

  let {
    slug,
    workId,
    liked,
    count,
    onChange,
  }: {
    slug: string;
    workId: string;
    liked: boolean;
    count: number;
    onChange: (liked: boolean, count: number) => void;
  } = $props();

  let busy = $state(false);
  let error = $state('');

  async function onClick() {
    if (busy) return;
    busy = true;
    error = '';
    try {
      const r = await toggleLike(slug, workId);
      onChange(r.liked, r.like_count);
    } catch (err) {
      if (err instanceof ApiError && err.status === 423) {
        error = '受付は終了しました';
      } else {
        error = err instanceof Error ? err.message : 'いいねできませんでした';
      }
    }
    busy = false;
  }

  const label = $derived(liked ? 'いいねを取り消す' : 'いいねする');
</script>

<div class="like-wrap">
  <button
    type="button"
    class="like-btn"
    class:liked
    disabled={busy}
    onclick={onClick}
    aria-pressed={liked}
    aria-label={label}
  >
    <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
      <path
        d="M12 21s-6.7-4.3-9.3-8.1C.6 9.7 2 5.6 5.6 4.7c2-.5 4 .3 5.2 2l1.2 1.6 1.2-1.6c1.2-1.7 3.2-2.5 5.2-2 3.6.9 5 5 2.9 8.2C18.7 16.7 12 21 12 21z"
        fill={liked ? 'currentColor' : 'none'}
        stroke="currentColor"
        stroke-width="1.8"
      />
    </svg>
    <span class="count">{count}</span>
    <span>いいね</span>
  </button>
  {#if error}<p class="error-text">{error}</p>{/if}
</div>

<style>
  .like-btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 10px 22px;
    min-height: 44px;
    border-radius: var(--radius);
    border: var(--hairline);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    font-weight: 700;
  }

  .like-btn:hover {
    background: var(--fill);
  }

  .like-btn.liked {
    color: var(--like);
    border: 1.5px solid var(--like);
  }

  .like-btn:disabled {
    opacity: 0.6;
    cursor: wait;
  }

  .count {
    font-size: 1rem;
  }
</style>
