<script lang="ts">
  import EventGuard from '../components/EventGuard.svelte';
  import LikeButton from '../components/LikeButton.svelte';
  import Lightbox from '../components/Lightbox.svelte';
  import {
    ApiError,
    fileUrl,
    getLikedWorkIds,
    getWork,
    type EventRecord,
    type WorkRecord,
  } from '../lib/api';
  import { getEditKey } from '../lib/keys';
  import { renderMarkdown } from '../lib/markdown';
  import { onLinkClick } from '../lib/router';

  let { slug, id }: { slug: string; id: string } = $props();

  let work = $state<WorkRecord | null>(null);
  let liked = $state(false);
  let loading = $state(true);
  let error = $state('');
  let lightboxIndex = $state(-1);
  let destroyed = false;
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  const hasEditKey = $derived(getEditKey(id) !== null);

  $effect(() => {
    return () => {
      destroyed = true;
      if (refetchTimer) clearTimeout(refetchTimer);
    };
  });

  async function load(event: EventRecord, reauth: () => void) {
    loading = true;
    error = '';
    try {
      work = await getWork(slug, id);
      try {
        const ids = await getLikedWorkIds(slug, event.id);
        liked = ids.includes(id);
      } catch {
        // いいね状態の復元失敗は無視
      }
      scheduleRefetch();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        reauth();
        return;
      }
      error =
        err instanceof ApiError && err.status === 404
          ? '作品が見つかりませんでした。削除された可能性があります。'
          : err instanceof Error
            ? err.message
            : 'エラーが発生しました';
    }
    loading = false;
  }

  /** 変換中は10秒間隔で再取得する */
  function scheduleRefetch() {
    if (refetchTimer) clearTimeout(refetchTimer);
    const status = work?.video_status;
    if (status !== 'processing' && status !== 'uploading') return;
    refetchTimer = setTimeout(async () => {
      if (destroyed) return;
      try {
        work = await getWork(slug, id);
      } catch {
        // 次の周回で再試行
      }
      scheduleRefetch();
    }, 10_000);
  }

  function youtubeId(url: string): string | null {
    try {
      const u = new URL(url);
      let id: string | null = null;
      if (u.hostname === 'youtu.be') {
        id = u.pathname.slice(1).split('/')[0] || null;
      } else if (/(^|\.)youtube(-nocookie)?\.com$/.test(u.hostname)) {
        if (u.pathname === '/watch') {
          id = u.searchParams.get('v');
        } else {
          const m = /^\/(embed|shorts|live)\/([^/?]+)/.exec(u.pathname);
          if (m) id = m[2];
        }
      }
      return id && /^[\w-]{5,20}$/.test(id) ? id : null;
    } catch {
      return null;
    }
  }

  const ytId = $derived(work?.video_url ? youtubeId(work.video_url) : null);
  const imageUrls = $derived(work ? work.images.map((name) => fileUrl(work!, name)) : []);
</script>

<EventGuard {slug} onReady={load}>
  {#snippet children({ event })}
    <p class="back">
      <a href={`/e/${slug}`} onclick={onLinkClick}>← 作品一覧へ</a>
    </p>

    {#if loading}
      <p class="status-msg">読み込み中…</p>
    {:else if error}
      <div class="error-box"><p>{error}</p></div>
    {:else if work}
      <article>
        <header class="work-head">
          <h1>{work.title}</h1>
          {#if hasEditKey}
            <a class="btn btn-sm" href={`/e/${slug}/w/${work.id}/edit`} onclick={onLinkClick}>編集</a>
          {/if}
        </header>

        {#if work.video_status === 'ready' && work.video}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video
            controls
            playsinline
            preload="metadata"
            poster={work.thumbnail ? fileUrl(work, work.thumbnail) : undefined}
            src={fileUrl(work, work.video)}
          ></video>
        {:else if work.video_status === 'processing' || work.video_status === 'uploading'}
          <div class="notice-box">動画は変換中です（数分かかります）。しばらくお待ちください。</div>
        {:else if work.video_status === 'failed'}
          <div class="error-box">
            <p>{work.video_error || '動画を変換できませんでした。'}</p>
          </div>
        {/if}

        {#if ytId}
          <div class="yt-wrap">
            <iframe
              src={`https://www.youtube-nocookie.com/embed/${ytId}`}
              title="動画"
              frameborder="0"
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
              allowfullscreen
            ></iframe>
          </div>
        {:else if work.video_url}
          <p>
            動画：<a href={work.video_url} target="_blank" rel="noopener nofollow">{work.video_url}</a>
          </p>
        {/if}

        {#if imageUrls.length > 0}
          <div class="gallery">
            {#each imageUrls as url, i (url)}
              <button
                type="button"
                class="gallery-item"
                onclick={() => (lightboxIndex = i)}
                aria-label={`画像 ${i + 1} を拡大`}
              >
                <img src={work.images[i] ? fileUrl(work, work.images[i], true) : url} alt="" loading="lazy" />
              </button>
            {/each}
          </div>
        {/if}

        {#if work.description}
          <div class="markdown-body description">
            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
            {@html renderMarkdown(work.description)}
          </div>
        {/if}

        {#if work.demo_url}
          <p class="demo">
            <a class="btn" href={work.demo_url} target="_blank" rel="noopener nofollow">
              デモを開く
            </a>
          </p>
        {/if}

        {#if work.tags.length > 0}
          <div class="tags">
            {#each work.tags as tag (tag)}
              <span class="badge tag">{tag}</span>
            {/each}
          </div>
        {/if}

        <div class="like-area">
          <LikeButton
            {slug}
            workId={work.id}
            {liked}
            count={work.like_count}
            onChange={(l, c) => {
              liked = l;
              if (work) work.like_count = c;
            }}
          />
        </div>

        <p class="posted">投稿日：{new Date(work.created).toLocaleDateString('ja-JP')}</p>
      </article>

      {#if lightboxIndex >= 0}
        <Lightbox images={imageUrls} start={lightboxIndex} onClose={() => (lightboxIndex = -1)} />
      {/if}
    {/if}
  {/snippet}
</EventGuard>

<style>
  .back {
    margin: 0 0 8px;
  }

  .work-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
  }

  .work-head h1 {
    margin: 0;
    font-size: 1.35rem;
  }

  video {
    width: 100%;
    border-radius: var(--radius);
    background: #000;
    margin-bottom: 12px;
  }

  .yt-wrap {
    position: relative;
    aspect-ratio: 16 / 9;
    margin-bottom: 12px;
  }

  .yt-wrap iframe {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    border-radius: var(--radius);
  }

  .gallery {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 8px;
    margin: 12px 0;
  }

  .gallery-item {
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
    cursor: zoom-in;
    background: var(--surface-2);
    aspect-ratio: 3 / 2;
  }

  .gallery-item img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .description {
    margin: 16px 0;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 12px 0;
  }

  .tag {
    background: var(--surface-2);
    color: var(--muted);
  }

  .like-area {
    margin: 20px 0 8px;
  }

  .posted {
    color: var(--muted);
    font-size: 0.85rem;
  }
</style>
