<script lang="ts">
  import { matchRoute, onLinkClick } from './lib/router';
  import EventSelect from './routes/EventSelect.svelte';
  import KeyManager from './routes/KeyManager.svelte';
  import WorkDetail from './routes/WorkDetail.svelte';
  import WorkForm from './routes/WorkForm.svelte';
  import WorkList from './routes/WorkList.svelte';

  let path = $state(location.pathname);
  const route = $derived(matchRoute(path));

  $effect(() => {
    const handler = () => {
      path = location.pathname;
      window.scrollTo(0, 0);
    };
    window.addEventListener('popstate', handler);
    return () => window.removeEventListener('popstate', handler);
  });

  const homeHref = $derived(route.params.slug ? `/e/${route.params.slug}` : '/');
</script>

<header class="topbar">
  <div class="topbar-inner">
    <a class="brand" href={homeHref} onclick={onLinkClick}>作品ひろば</a>
    <a class="keys-link" href="/keys" onclick={onLinkClick}>編集キーを管理</a>
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
      <div class="notfound">
        <h1>ページが見つかりません</h1>
        <a class="btn" href="/" onclick={onLinkClick}>トップへ戻る</a>
      </div>
    {/if}
  {/key}
</main>

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

  .brand {
    font-weight: 800;
    font-size: 1.05rem;
    letter-spacing: 0.01em;
    color: var(--text);
  }

  .brand:hover {
    text-decoration: none;
  }

  .keys-link {
    font-size: 0.85rem;
    font-weight: 700;
    color: var(--text);
  }

  .notfound {
    text-align: center;
    padding: 48px 16px;
  }

  .notfound h1 {
    font-size: 1.2rem;
  }
</style>
