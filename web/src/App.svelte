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
    <a class="gear" href="/keys" onclick={onLinkClick} aria-label="編集キーを管理" title="編集キーを管理">
      <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
        <path
          d="M12 15.5a3.5 3.5 0 1 1 0-7 3.5 3.5 0 0 1 0 7zm7.4-2.6c.05-.3.08-.6.08-.9s-.03-.6-.08-.9l2-1.6a.5.5 0 0 0 .12-.63l-1.9-3.3a.5.5 0 0 0-.6-.22l-2.37.95a7.3 7.3 0 0 0-1.55-.9l-.36-2.52a.5.5 0 0 0-.5-.42h-3.8a.5.5 0 0 0-.5.42l-.36 2.52c-.56.23-1.08.53-1.55.9l-2.37-.95a.5.5 0 0 0-.6.22l-1.9 3.3a.5.5 0 0 0 .12.63l2 1.6c-.05.3-.08.6-.08.9s.03.6.08.9l-2 1.6a.5.5 0 0 0-.12.63l1.9 3.3c.13.22.39.31.6.22l2.37-.95c.47.37.99.67 1.55.9l.36 2.52c.04.24.25.42.5.42h3.8c.25 0 .46-.18.5-.42l.36-2.52c.56-.23 1.08-.53 1.55-.9l2.37.95c.21.09.47 0 .6-.22l1.9-3.3a.5.5 0 0 0-.12-.63l-2-1.6z"
          fill="currentColor"
        />
      </svg>
    </a>
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
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }

  .topbar-inner {
    max-width: 1100px;
    margin: 0 auto;
    padding: 10px 16px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .brand {
    font-weight: 800;
    font-size: 1.05rem;
    color: var(--text);
  }

  .brand:hover {
    text-decoration: none;
  }

  .gear {
    color: var(--muted);
    display: flex;
    align-items: center;
    padding: 8px;
    border-radius: 999px;
  }

  .gear:hover {
    color: var(--text);
    background: var(--surface-2);
  }

  .notfound {
    text-align: center;
    padding: 48px 16px;
  }

  .notfound h1 {
    font-size: 1.2rem;
  }
</style>
