<script lang="ts">
  import EventGuard from '../components/EventGuard.svelte';
  import WorkCard from '../components/WorkCard.svelte';
  import {
    ApiError,
    getLikedWorkIds,
    listWorks,
    type EventRecord,
    type WorkRecord,
  } from '../lib/api';
  import { errMsg } from '../lib/errors';
  import { renderMarkdown } from '../lib/markdown';
  import { onLinkClick } from '../lib/router';

  let { slug }: { slug: string } = $props();

  let works = $state<WorkRecord[]>([]);
  let likedIds = $state<Set<string>>(new Set());
  let loading = $state(true);
  let error = $state('');
  let sortBy = $state<'new' | 'likes'>('new');
  let query = $state('');

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    let list = q ? works.filter((w) => w.description.toLowerCase().includes(q)) : works.slice();
    if (sortBy === 'likes') {
      list = list
        .slice()
        .sort((a, b) => b.like_count - a.like_count || b.created.localeCompare(a.created));
    }
    return list;
  });

  async function load(event: EventRecord, reauth: () => void) {
    loading = true;
    error = '';
    try {
      works = await listWorks(slug, event.id);
      try {
        likedIds = new Set(await getLikedWorkIds(slug, event.id));
      } catch {
        // いいね状態の復元失敗は致命的でないので無視
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        reauth();
        return;
      }
      error = errMsg(err);
    }
    loading = false;
  }
</script>

<EventGuard {slug} onReady={load}>
  {#snippet children({ event })}
    <div class="head-row">
      <div class="head-text">
        <h1>{event.name}</h1>
        {#if event.description}
          <div class="markdown-body event-desc">
            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
            {@html renderMarkdown(event.description)}
          </div>
        {/if}
      </div>
      {#if event.submissions_open}
        <a class="btn btn-primary" href={`/e/${slug}/new`} onclick={onLinkClick}>投稿する</a>
      {:else}
        <span class="badge closed">受付終了</span>
      {/if}
    </div>

    <div class="controls">
      <input
        type="search"
        class="search"
        placeholder="説明文で検索"
        bind:value={query}
        aria-label="説明文で検索"
      />
      <nav class="sort" aria-label="並び替え">
        <button
          type="button"
          class="sort-btn"
          class:active={sortBy === 'new'}
          aria-pressed={sortBy === 'new'}
          onclick={() => (sortBy = 'new')}
        >
          新着順
        </button>
        <button
          type="button"
          class="sort-btn"
          class:active={sortBy === 'likes'}
          aria-pressed={sortBy === 'likes'}
          onclick={() => (sortBy = 'likes')}
        >
          いいね順
        </button>
      </nav>
    </div>

    {#if loading}
      <p class="status-msg">読み込み中…</p>
    {:else if error}
      <div class="error-box">
        <p>{error}</p>
        <button type="button" class="btn btn-sm" onclick={() => load(event, () => location.reload())}>
          再読み込み
        </button>
      </div>
    {:else if filtered.length === 0}
      <div class="empty">
        {#if works.length === 0}
          <p class="status-msg">まだ作品がありません。</p>
          {#if event.submissions_open}
            <p class="empty-action">
              <a href={`/e/${slug}/new`} onclick={onLinkClick}>最初の作品を投稿する</a>
            </p>
          {/if}
        {:else}
          <p class="status-msg">一致する作品がありません。</p>
        {/if}
      </div>
    {:else}
      <div class="grid">
        {#each filtered as work (work.id)}
          <WorkCard {work} {slug} liked={likedIds.has(work.id)} />
        {/each}
      </div>
    {/if}
  {/snippet}
</EventGuard>

<style>
  /*
   * 余白の設計(8pxスケール):
   *   見出しブロック → コントロール行: 40px / コントロール行 → グリッド: 24px
   *   カード間: 24px。検索とソートは同じ高さラインに揃える
   */
  .head-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 32px;
  }

  .head-row h1 {
    margin: 0;
    font-size: 1.6rem;
  }

  .head-row .btn {
    flex-shrink: 0;
    margin-top: 4px; /* h1 のキャップハイトに視覚的に揃える */
  }

  .badge.closed {
    flex-shrink: 0;
    margin-top: 8px;
    background: var(--fill);
    color: var(--muted);
    border: none;
    padding: 8px 16px;
  }

  .event-desc {
    margin-top: 8px;
    font-size: 0.95rem;
    max-width: 640px;
  }

  .event-desc :global(p) {
    margin: 0;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 24px;
    margin: 40px 0 24px;
  }

  .controls .search {
    flex: 1;
    max-width: 480px;
    min-height: 44px;
  }

  .sort {
    display: inline-flex;
    border: var(--hairline);
    border-radius: var(--radius);
    overflow: hidden;
    margin-left: auto;
  }

  .sort-btn {
    background: var(--surface);
    border: none;
    padding: 0 18px;
    min-height: 42px;
    font-size: 0.85rem;
    font-weight: 400;
    color: var(--muted);
    cursor: pointer;
    transition: background var(--transition-fast), color var(--transition-fast);
  }

  .sort-btn:hover:not(.active) {
    background: var(--fill);
    color: var(--text);
  }

  .sort-btn + .sort-btn {
    border-left: var(--hairline);
  }

  .sort-btn.active {
    background: var(--color-primary);
    color: var(--color-primary-contrast);
    font-weight: 700;
  }

  .grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 24px;
  }

  @media (min-width: 560px) {
    .grid {
      grid-template-columns: repeat(2, 1fr);
    }
  }

  @media (min-width: 880px) {
    .grid {
      grid-template-columns: repeat(3, 1fr);
    }
  }

  @media (max-width: 559px) {
    .head-row {
      flex-direction: column;
      gap: 16px;
    }

    .head-row .btn {
      margin-top: 0;
      align-self: stretch;
    }

    .controls {
      flex-direction: column;
      align-items: stretch;
      gap: 12px;
    }

    .controls .search {
      max-width: none;
    }

    .sort {
      margin-left: 0;
      align-self: flex-start;
    }
  }

  .empty {
    text-align: center;
    padding: 64px 16px;
  }

  .empty .status-msg {
    padding: 0 0 8px;
  }

  .empty-action {
    margin: 0;
    font-size: 0.9rem;
  }
</style>
