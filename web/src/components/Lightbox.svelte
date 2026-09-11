<script lang="ts">
  let {
    images,
    start = 0,
    onClose,
  }: {
    images: string[];
    start?: number;
    onClose: () => void;
  } = $props();

  // svelte-ignore state_referenced_locally -- start は初期表示位置としてのみ使う
  let index = $state(start);

  function prev() {
    index = (index - 1 + images.length) % images.length;
  }

  function next() {
    index = (index + 1) % images.length;
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose();
    else if (e.key === 'ArrowLeft' && images.length > 1) prev();
    else if (e.key === 'ArrowRight' && images.length > 1) next();
  }

  function onOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose();
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  class="lightbox"
  role="dialog"
  aria-modal="true"
  aria-label="画像の拡大表示"
  tabindex="-1"
  onclick={onOverlayClick}
>
  <img src={images[index]} alt="" />
  <button type="button" class="close" onclick={onClose} aria-label="閉じる">×</button>
  {#if images.length > 1}
    <button type="button" class="nav prev" onclick={prev} aria-label="前の画像">‹</button>
    <button type="button" class="nav next" onclick={next} aria-label="次の画像">›</button>
    <span class="counter">{index + 1} / {images.length}</span>
  {/if}
</div>

<style>
  .lightbox {
    position: fixed;
    inset: 0;
    z-index: 200;
    background: rgba(0, 0, 0, 0.88);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .lightbox img {
    max-width: 96vw;
    max-height: 92vh;
    object-fit: contain;
  }

  .lightbox button {
    position: absolute;
    background: rgba(255, 255, 255, 0.12);
    color: #fff;
    border: none;
    border-radius: 999px;
    width: 44px;
    height: 44px;
    font-size: 1.5rem;
    line-height: 1;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .close {
    top: 14px;
    right: 14px;
  }

  .nav {
    top: 50%;
    transform: translateY(-50%);
  }

  .nav.prev {
    left: 10px;
  }

  .nav.next {
    right: 10px;
  }

  .counter {
    position: absolute;
    bottom: 16px;
    left: 50%;
    transform: translateX(-50%);
    color: #fff;
    font-size: 0.85rem;
    background: rgba(0, 0, 0, 0.5);
    padding: 2px 12px;
    border-radius: 999px;
  }
</style>
