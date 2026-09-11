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
    <div class="list-head">
      <div class="head-row">
        <h1><span class="hl">{event.name}</span></h1>
        {#if event.submissions_open}
          <a class="btn btn-primary" href={`/e/${slug}/new`} onclick={onLinkClick}>投稿する</a>
        {:else}
          <span class="badge closed">受付終了</span>
        {/if}
      </div>
      {#if event.description}
        <div class="markdown-body event-desc">
          <!-- eslint-disable-next-line svelte/no-at-html-tags -->
          {@html renderMarkdown(event.description)}
        </div>
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
      <select bind:value={sortBy} aria-label="並び替え">
        <option value="new">新着順</option>
        <option value="likes">いいね順</option>
      </select>
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
  .list-head h1 {
    margin: 0;
    font-size: 1.45rem;
  }

  .head-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 8px;
  }

  .badge.closed {
    background: var(--surface-2);
    color: var(--muted);
    padding: 8px 16px;
    border-style: dashed;
  }

  .event-desc {
    color: var(--muted);
    font-size: 0.92rem;
    margin-bottom: 12px;
  }

  .controls {
    display: flex;
    gap: 8px;
    margin: 12px 0 16px;
  }

  .controls .search {
    flex: 1;
  }

  .controls select {
    width: auto;
    flex-shrink: 0;
  }

  .grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 18px;
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

  @media (min-width: 1200px) {
    .grid {
      grid-template-columns: repeat(4, 1fr);
    }
  }
</style>
