<script lang="ts">
  import { ApiError, resolveEvent } from '../lib/api';
  import { errMsg } from '../lib/errors';
  import { getEventKeys, removeEventKey, setEventKey } from '../lib/keys';
  import { navigate } from '../lib/router';

  const knownSlugs = Object.keys(getEventKeys());
  const redirectTo = knownSlugs.length > 0 ? knownSlugs[knownSlugs.length - 1] : null;

  $effect(() => {
    if (redirectTo) navigate(`/e/${redirectTo}`, { replace: true });
  });

  let slug = $state('');
  let passphrase = $state('');
  let busy = $state(false);
  let error = $state('');

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    const s = slug.trim().toLowerCase();
    if (!/^[a-z0-9-]{2,40}$/.test(s)) {
      error = 'イベントIDは半角英小文字・数字・ハイフン（2〜40文字）で入力してください';
      return;
    }
    const p = passphrase.trim();
    if (!p) {
      error = '合言葉を入力してください';
      return;
    }
    busy = true;
    error = '';
    setEventKey(s, p);
    try {
      await resolveEvent(s);
      navigate(`/e/${s}`);
    } catch (err) {
      removeEventKey(s);
      if (err instanceof ApiError && (err.status === 400 || err.status === 401 || err.status === 403)) {
        error = 'イベントIDまたは合言葉が違います';
      } else {
        error = errMsg(err);
      }
      busy = false;
    }
  }
</script>

{#if redirectTo}
  <p class="status-msg">移動中…</p>
{:else}
  <div class="select-wrap">
    <form class="card select-card" onsubmit={submit}>
      <h1>作品ひろば</h1>
      <p class="hint">先生から配られたイベントIDと合言葉を入力してください。</p>
      <div class="field">
        <label for="ev-slug">イベントID</label>
        <input
          id="ev-slug"
          type="text"
          bind:value={slug}
          placeholder="例: fes2026"
          autocapitalize="off"
          autocomplete="off"
          spellcheck="false"
          disabled={busy}
        />
      </div>
      <div class="field">
        <label for="ev-pass">合言葉</label>
        <input
          id="ev-pass"
          type="text"
          bind:value={passphrase}
          placeholder="合言葉"
          autocapitalize="off"
          autocomplete="off"
          spellcheck="false"
          disabled={busy}
        />
      </div>
      {#if error}<p class="error-text">{error}</p>{/if}
      <button type="submit" class="btn btn-primary" disabled={busy}>
        {busy ? '確認中…' : '入る'}
      </button>
    </form>
  </div>
{/if}

<style>
  .select-wrap {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: calc(100dvh - 300px);
  }

  .select-card {
    width: 100%;
    max-width: 420px;
    padding: 32px 28px;
  }

  .select-card h1 {
    margin: 0 0 8px;
    font-size: 1.4rem;
  }

  .select-card .btn {
    width: 100%;
    margin-top: 16px;
  }
</style>
