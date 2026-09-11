<script lang="ts">
  import { fileUrl, type WorkRecord } from '../lib/api';
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
    <h3 class="title">{work.title}</h3>
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
    overflow: hidden;
    color: inherit;
    transition:
      transform 0.1s ease,
      box-shadow 0.1s ease;
  }

  .work-card:hover {
    text-decoration: none;
    transform: translate(-2px, -2px);
    box-shadow: 6px 6px 0 var(--shadow-c);
  }

  .thumb {
    position: relative;
    aspect-ratio: 3 / 2;
    background: var(--surface-2);
  }

  .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .placeholder {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--muted);
    font-size: 0.85rem;
  }

  .overlay-badge {
    position: absolute;
    top: 8px;
    left: 8px;
  }

  .overlay-badge.video {
    background: var(--text);
    color: var(--bg);
    border-color: var(--border);
  }

  .overlay-badge.processing {
    background: var(--warn-bg);
    color: var(--warn-text);
  }

  .meta {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 8px;
    padding: 10px 12px;
    border-top: 2px solid var(--border);
  }

  .title {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 800;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .likes {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--muted);
    font-size: 0.9rem;
    flex-shrink: 0;
    padding-top: 2px;
  }

  .likes.liked {
    color: var(--like);
  }
</style>
