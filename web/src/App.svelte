<script lang="ts">
  import { onDestroy } from 'svelte';
  import { matchRoute, onLinkClick, subscribePath } from './lib/router';
  import EventSelect from './routes/EventSelect.svelte';
  import KeyManager from './routes/KeyManager.svelte';
  import WorkDetail from './routes/WorkDetail.svelte';
  import WorkForm from './routes/WorkForm.svelte';
  import WorkList from './routes/WorkList.svelte';

  let path = $state(location.pathname);
  const route = $derived(matchRoute(path));

  // 子の $effect より先に購読する。$effect 内だと初回 navigate を取りこぼす。
  onDestroy(
    subscribePath((next) => {
      path = next;
      window.scrollTo(0, 0);
    }),
  );

  const homeHref = $derived(route.params.slug ? `/e/${route.params.slug}` : '/');
</script>

<header class="topbar">
  <div class="topbar-inner">
    <div class="brand-area">
      <a class="brand" href={homeHref} onclick={onLinkClick}>NccHub</a>
      {#if route.params.slug}
        <span class="crumb">/ {route.params.slug}</span>
      {/if}
    </div>
    <a class="keys-link" class:active={path === '/keys'} href="/keys" onclick={onLinkClick}
      >編集キーを管理</a
    >
  </div>
</header>

<main class="container">
  {#key path}
    {#if route.name === 'home'}
      <EventSelect />
    {:else if route.name === 'keys'}
      <KeyManager />
    {:else if route.name === 'list'}
      <WorkList slug={route.params.slug} />
    {:else if route.name === 'new'}
      <WorkForm slug={route.params.slug} />
    {:else if route.name === 'detail'}
      <WorkDetail slug={route.params.slug} id={route.params.id} />
    {:else if route.name === 'edit'}
      <WorkForm slug={route.params.slug} id={route.params.id} />
    {:else}
      <div class="stage-center">
        <div class="notfound">
          <h1>ページが見つかりません</h1>
          <p class="meta">URLが間違っているか、ページが移動した可能性があります。</p>
          <a class="btn" href="/" onclick={onLinkClick}>トップへ戻る</a>
        </div>
      </div>
    {/if}
  {/key}
</main>

<footer class="footer">
  <div class="footer-inner">
    <span class="meta">NccHub</span>
  </div>
</footer>

<style>
  .topbar {
    position: sticky;
    top: 0;
    z-index: 50;
    background: var(--bg);
    border-bottom: var(--hairline);
  }

  /* 版面: .container と同じ最大幅・左右余白に揃える */
  .topbar-inner {
    max-width: 1060px;
    height: 56px;
    margin: 0 auto;
    padding: 0 20px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .brand-area {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }

  .brand {
    font-weight: 800;
    font-size: 1.05rem;
    letter-spacing: 0.01em;
    color: var(--text);
    transition: color var(--transition-fast);
  }

  .brand:hover {
    color: var(--color-primary);
    text-decoration: none;
  }

  .crumb {
    color: var(--muted);
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  @media (max-width: 420px) {
    .crumb {
      display: none;
    }
  }

  .keys-link {
    font-size: 0.85rem;
    font-weight: 700;
    color: var(--muted);
    transition: color var(--transition-fast);
  }

  .keys-link:hover {
    color: var(--text);
  }

  .keys-link.active {
    color: var(--color-primary-text);
    text-decoration: underline;
    text-underline-offset: 4px;
  }

  .footer {
    border-top: 0.5px solid var(--border-soft);
  }

  .footer-inner {
    max-width: 1060px;
    margin: 0 auto;
    padding: 20px 20px 28px;
  }

  .notfound {
    text-align: center;
    padding: 48px 16px;
  }

  .notfound h1 {
    font-size: 1.2rem;
  }

  .notfound .meta {
    margin: 12px 0 24px;
  }
</style>
