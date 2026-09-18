<script lang="ts">
  import { copyText } from '../lib/clipboard';
  import {
    getEditKeys,
    getEventKeys,
    getWorkCodes,
    removeEditKey,
    removeEventKey,
    removeWorkCode,
    setEditKey,
  } from '../lib/keys';

  let editKeys = $state<Record<string, string>>(getEditKeys());
  let eventKeys = $state<Record<string, string>>(getEventKeys());
  let workCodes = $state<Record<string, string>>(getWorkCodes());

  let newWorkId = $state('');
  let newKey = $state('');
  let addError = $state('');
  let copiedId = $state('');
  let copyFailedId = $state('');

  function refresh() {
    editKeys = getEditKeys();
    eventKeys = getEventKeys();
    workCodes = getWorkCodes();
  }

  function onRemoveEditKey(workId: string) {
    if (!confirm('この編集キーと作品コードを端末から削除しますか？（作品自体は消えません）')) return;
    removeEditKey(workId);
    removeWorkCode(workId);
    refresh();
  }

  function onRemoveEventKey(slug: string) {
    if (!confirm(`イベント「${slug}」の合言葉を端末から削除しますか？`)) return;
    removeEventKey(slug);
    refresh();
  }

  function addKey(e: SubmitEvent) {
    e.preventDefault();
    addError = '';
    const workId = newWorkId.trim();
    const key = newKey.trim();
    if (!workId || !key) {
      addError = '作品IDと編集キーの両方を入力してください';
      return;
    }
    setEditKey(workId, key);
    newWorkId = '';
    newKey = '';
    refresh();
  }

  async function copy(target: string, value: string) {
    copyFailedId = '';
    if (await copyText(value)) {
      copiedId = target;
      setTimeout(() => (copiedId = ''), 2000);
    } else {
      copyFailedId = target;
      setTimeout(() => (copyFailedId = ''), 2000);
    }
  }

  const editEntries = $derived(Object.entries(editKeys));
  const eventEntries = $derived(Object.entries(eventKeys));
</script>

<h1>編集キーを管理</h1>
<p class="hint">
  この端末に保存されている鍵の一覧です。別の端末で投稿した作品を編集したいときは、編集キーを手入力で追加できます。
  作品コードはアンケートに書くときに使います。
</p>

<section class="card section">
  <h2>編集キー（作品ごと）</h2>
  {#if editEntries.length === 0}
    <p class="hint">保存されている編集キーはありません。</p>
  {:else}
    <ul class="key-list">
      {#each editEntries as [workId, key] (workId)}
        <li>
          <div class="key-info">
            <span class="work-id">作品ID：{workId}</span>
            {#if workCodes[workId]}
              <span class="code-row">
                作品コード：<code class="work-code">{workCodes[workId]}</code>
                <button
                  type="button"
                  class="btn btn-sm"
                  onclick={() => copy(workId + ':code', workCodes[workId])}
                >
                  {copiedId === workId + ':code'
                    ? 'コピーしました'
                    : copyFailedId === workId + ':code'
                      ? 'コピーできません'
                      : 'コピー'}
                </button>
              </span>
            {/if}
            <code>{key}</code>
          </div>
          <div class="key-actions">
            <button type="button" class="btn btn-sm" onclick={() => copy(workId, key)}>
              {copiedId === workId
                ? 'コピーしました'
                : copyFailedId === workId
                  ? 'コピーできません'
                  : '編集キーをコピー'}
            </button>
            <button type="button" class="btn btn-sm btn-danger" onclick={() => onRemoveEditKey(workId)}>
              削除
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}

  <form class="add-form" onsubmit={addKey}>
    <h3>編集キーを手入力で追加</h3>
    <div class="add-row">
      <input type="text" bind:value={newWorkId} placeholder="作品ID" autocapitalize="off" spellcheck="false" />
      <input type="text" bind:value={newKey} placeholder="編集キー" autocapitalize="off" spellcheck="false" />
      <button type="submit" class="btn btn-sm">追加</button>
    </div>
    <p class="hint">作品IDは作品ページのURL（/w/ の後ろ）で確認できます。</p>
    {#if addError}<p class="error-text">{addError}</p>{/if}
  </form>
</section>

<section class="card section">
  <h2>合言葉（イベントごと）</h2>
  {#if eventEntries.length === 0}
    <p class="hint">保存されている合言葉はありません。</p>
  {:else}
    <ul class="key-list">
      {#each eventEntries as [slug] (slug)}
        <li>
          <div class="key-info">
            <span class="work-id">イベント：{slug}</span>
            <code>••••••••</code>
          </div>
          <div class="key-actions">
            <button type="button" class="btn btn-sm btn-danger" onclick={() => onRemoveEventKey(slug)}>
              削除
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  h1 {
    font-size: 1.5rem;
    margin: 0 0 8px;
  }

  .section {
    max-width: 720px;
    padding: 24px;
    margin: 24px 0 0;
  }

  .section h2 {
    font-size: 1.05rem;
    margin: 0 0 16px;
  }

  .key-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .key-list li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    flex-wrap: wrap;
    background: var(--fill);
    border-radius: var(--radius);
    padding: 12px 16px;
  }

  .key-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .work-id {
    font-size: 13px;
    color: var(--muted);
  }

  .key-info code {
    word-break: break-all;
    font-size: 0.9rem;
  }

  .code-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    font-size: 13px;
    color: var(--muted);
    margin: 2px 0;
  }

  .code-row .work-code {
    font-size: 1rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    color: var(--text);
  }

  .key-actions {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
  }

  .add-form {
    margin-top: 24px;
    border-top: var(--hairline);
    padding-top: 20px;
  }

  .add-form h3 {
    font-size: 0.95rem;
    margin: 0 0 8px;
  }

  .add-row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .add-row input {
    flex: 1;
    min-width: 140px;
  }

  .add-row .btn {
    flex-shrink: 0;
  }
</style>
