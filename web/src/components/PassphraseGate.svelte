<script lang="ts">
  import { ApiError, resolveEvent, type EventRecord } from '../lib/api';
  import { errMsg } from '../lib/errors';
  import { removeEventKey, setEventKey } from '../lib/keys';

  let {
    slug,
    onSuccess,
  }: {
    slug: string;
    onSuccess: (event: EventRecord) => void;
  } = $props();

  let passphrase = $state('');
  let busy = $state(false);
  let error = $state('');

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    const value = passphrase.trim();
    if (!value) {
      error = '合言葉を入力してください';
      return;
    }
    busy = true;
    error = '';
    setEventKey(slug, value);
    try {
      const ev = await resolveEvent(slug);
      onSuccess(ev);
    } catch (err) {
      removeEventKey(slug);
      if (err instanceof ApiError && (err.status === 400 || err.status === 401 || err.status === 403)) {
        error = '合言葉が違います';
      } else {
        error = errMsg(err);
      }
    }
    busy = false;
  }
</script>

<div class="overlay" role="dialog" aria-modal="true" aria-label="合言葉の入力">
  <form class="gate card" onsubmit={submit}>
    <h2>合言葉を入力</h2>
    <p class="hint">
      このイベント（{slug}）を見るには合言葉が必要です。先生から配られた合言葉を入力してください。
    </p>
    <input
      type="text"
      bind:value={passphrase}
      placeholder="合言葉"
      autocomplete="off"
      autocapitalize="off"
      spellcheck="false"
      disabled={busy}
    />
    {#if error}<p class="error-text">{error}</p>{/if}
    <button type="submit" class="btn btn-primary" disabled={busy}>
      {busy ? '確認中…' : '入る'}
    </button>
  </form>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 100;
    background: rgba(17, 17, 17, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }

  .gate {
    width: 100%;
    max-width: 420px;
    padding: 32px 28px;
    display: flex;
    flex-direction: column;
    gap: 16px;
    background: var(--bg);
  }

  .gate h2 {
    margin: 0;
    font-size: 1.25rem;
  }

  .gate .hint {
    margin: 0;
  }
</style>
