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
    let list = q ? works.filter((w) => w.title.toLowerCase().includes(q)) : works.slice();
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
      error = err instanceof Error ? err.message : 'エラーが発生しました';
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
        placeholder="タイトルで検索"
        bind:value={query}
        aria-label="タイトルで検索"
      />
      <nav class="sort" aria-label="並び替え">
        <button type="button" class="sort-btn" class:active={sortBy === 'new'} onclick={() => (sortBy = 'new')}>
          新着順
        </button>
        <button type="button" class="sort-btn" class:active={sortBy === 'likes'} onclick={() => (sortBy = 'likes')}>
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
      <p class="status-msg">
        {works.length === 0 ? 'まだ作品がありません。最初の投稿をしてみましょう。' : '一致する作品がありません。'}
      </p>
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
   *   見出しブロック → コントロール行: 32px / コントロール行 → グリッド: 24px
   *   カード間: 24px。検索とソートは同じ 44px の高さラインに揃える
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
  }

  .event-desc :global(p) {
    margin: 0;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 24px;
    margin: 32px 0 24px;
  }

  .controls .search {
    flex: 1;
    max-width: 480px;
    min-height: 44px;
  }

  .sort {
    display: flex;
    gap: 20px;
    margin-left: auto;
  }

  .sort-btn {
    background: none;
    border: none;
    padding: 0;
    font-size: 0.9rem;
    font-weight: 400;
    color: var(--muted);
    cursor: pointer;
  }

  .sort-btn.active {
    font-weight: 700;
    color: var(--text);
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
    }
  }
</style>
