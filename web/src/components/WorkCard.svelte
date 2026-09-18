<script lang="ts">
  import { fileUrl, type WorkRecord } from '../lib/api';
  import { plainExcerpt } from '../lib/markdown';
  import { onLinkClick } from '../lib/router';

  let {
    work,
    slug,
    liked = false,
  }: {
    work: WorkRecord;
    slug: string;
    liked?: boolean;
  } = $props();

  const thumb = $derived(
    work.thumbnail
      ? fileUrl(work, work.thumbnail)
      : work.images.length > 0
        ? fileUrl(work, work.images[0], true)
        : null,
  );
  // タイトル欄が無いので説明の冒頭を見出しにする（SPEC §9.1）
  const heading = $derived(plainExcerpt(work.description) || '（説明なし）');
  const converting = $derived(work.video_status === 'processing' || work.video_status === 'uploading');
  const hasVideo = $derived(work.video_status === 'ready' || !!work.video_url);
</script>

<a class="card work-card" href={`/e/${slug}/w/${work.id}`} onclick={onLinkClick}>
  <div class="thumb">
    {#if thumb}
      <img src={thumb} alt="" loading="lazy" />
    {:else}
      <div class="placeholder" aria-hidden="true">画像なし</div>
    {/if}
    {#if converting}
      <span class="badge overlay-badge processing">変換中</span>
    {:else if hasVideo}
      <span class="badge overlay-badge video">動画</span>
    {/if}
  </div>
  <div class="meta">
    <h3 class="title">{heading}</h3>
    <span class="likes" class:liked aria-label={`いいね ${work.like_count}件`}>
      <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
        <path
          d="M12 21s-6.7-4.3-9.3-8.1C.6 9.7 2 5.6 5.6 4.7c2-.5 4 .3 5.2 2l1.2 1.6 1.2-1.6c1.2-1.7 3.2-2.5 5.2-2 3.6.9 5 5 2.9 8.2C18.7 16.7 12 21 12 21z"
          fill={liked ? 'currentColor' : 'none'}
          stroke="currentColor"
          stroke-width="1.8"
        />
      </svg>
      {work.like_count}
    </span>
  </div>
</a>

<style>
  .work-card {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    color: inherit;
    transition: background var(--transition-fast);
  }

  .work-card:hover {
    text-decoration: none;
    background: var(--fill);
  }

  /*
   * サムネイル枠は全カード同じ 3:2（横長）にする。
   * 画像の固有サイズで枠が伸びないよう、中身は絶対配置する。
   */
  .thumb {
    position: relative;
    aspect-ratio: 3 / 2;
    flex-shrink: 0;
    overflow: hidden;
    background: var(--fill);
  }

  .thumb img,
  .placeholder {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }

  .thumb img {
    object-fit: cover;
    display: block;
  }

  .placeholder {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--muted);
    font-size: 0.85rem;
  }

  .overlay-badge {
    position: absolute;
    top: 12px;
    left: 12px;
    border: none;
    border-radius: var(--radius-sm);
  }

  .overlay-badge.video {
    background: var(--color-secondary);
    color: var(--color-secondary-contrast);
  }

  .overlay-badge.processing {
    background: var(--color-surface);
    color: var(--color-warning);
    border: 1px solid var(--color-warning);
  }

  /* タイトルといいね数は同一ベースラインに揃える */
  .meta {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
    padding: 12px 16px 14px;
    border-top: var(--hairline);
  }

  .title {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 700;
    line-height: 1.35;
    min-height: calc(1.35em * 2);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .likes {
    display: inline-flex;
    align-items: baseline;
    gap: 4px;
    color: var(--muted);
    font-size: 13px;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }

  .likes.liked {
    color: var(--like);
  }
</style>
