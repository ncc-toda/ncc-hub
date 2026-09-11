<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ApiError, resolveEvent, type EventRecord } from '../lib/api';
  import { getEventKey } from '../lib/keys';
  import PassphraseGate from './PassphraseGate.svelte';

  let {
    slug,
    onReady,
    children,
  }: {
    slug: string;
    /** イベント解決成功時に呼ばれる。reauth() を呼ぶと合言葉ゲートを再表示する。 */
    onReady?: (event: EventRecord, reauth: () => void) => void;
    children: Snippet<[{ event: EventRecord; reauth: () => void }]>;
  } = $props();

  let event = $state<EventRecord | null>(null);
  let loading = $state(true);
  let needKey = $state(false);
  let errorMessage = $state('');

  function reauth() {
    event = null;
    needKey = true;
  }

  function ready(ev: EventRecord) {
    event = ev;
    needKey = false;
    loading = false;
    onReady?.(ev, reauth);
  }

  async function load() {
    loading = true;
    errorMessage = '';
    needKey = false;
    if (!getEventKey(slug)) {
      needKey = true;
      loading = false;
      return;
    }
    try {
      ready(await resolveEvent(slug));
    } catch (err) {
      if (err instanceof ApiError && (err.status === 400 || err.status === 401 || err.status === 403)) {
        needKey = true;
      } else {
        errorMessage = err instanceof Error ? err.message : 'エラーが発生しました';
      }
      loading = false;
    }
  }

  load();
</script>

{#if needKey}
  <PassphraseGate {slug} onSuccess={ready} />
{:else if loading}
  <p class="status-msg">読み込み中…</p>
{:else if errorMessage}
  <div class="error-box">
    <p>{errorMessage}</p>
    <button type="button" class="btn btn-sm" onclick={load}>再読み込み</button>
  </div>
{:else if event}
  {@render children({ event, reauth })}
{/if}
