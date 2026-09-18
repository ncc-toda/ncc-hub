<script lang="ts">
  import { tick } from 'svelte';
  import EventGuard from '../components/EventGuard.svelte';
  import UploadProgress from '../components/UploadProgress.svelte';
  import {
    ApiError,
    createWork,
    deleteWork,
    fileUrl,
    getWork,
    updateWork,
    videoCancel,
    type EventRecord,
    type WorkLink,
    type WorkRecord,
  } from '../lib/api';
  import { copyText } from '../lib/clipboard';
  import { errMsg } from '../lib/errors';
  import { formatGB } from '../lib/format';
  import { IMAGE_ACCEPT, processImage } from '../lib/image';
  import {
    getEditKey,
    getUpload,
    removeEditKey,
    removeUpload,
    removeWorkCode,
    setEditKey,
    setWorkCode,
    type StoredUpload,
  } from '../lib/keys';
  import { renderMarkdown } from '../lib/markdown';
  import { navigate, onLinkClick } from '../lib/router';
  import {
    fileExt,
    pollVideoStatus,
    VideoUploader,
    VIDEO_EXTS,
    type UploadSnapshot,
  } from '../lib/upload';

  let { slug, id = '' }: { slug: string; id?: string } = $props();
  // ルートは path で {#key} され毎回作り直されるので、初期値の取り込みで問題ない
  // svelte-ignore state_referenced_locally
  const isEdit = id !== '';

  type Step = 'loading' | 'needkey' | 'form' | 'editkey' | 'upload' | 'processing' | 'done';

  let event = $state<EventRecord | null>(null);
  let step = $state<Step>('loading');
  let loadError = $state('');

  // --- フォーム項目 ---
  let description = $state('');
  let showPreview = $state(false);
  let videoFile = $state<File | null>(null);
  let videoUrl = $state('');
  let demoUrl = $state('');
  let githubUrl = $state('');
  let tagsText = $state('');
  let links = $state<WorkLink[]>([]);

  interface NewImage {
    file: File;
    url: string;
  }
  let newImages = $state<NewImage[]>([]);
  let existingImages = $state<{ name: string; removed: boolean }[]>([]);
  let imageError = $state('');
  let imageBusy = $state(false);
  let dragOver = $state(false);
  let imageSeq = 0;

  let work = $state<WorkRecord | null>(null);
  let editKey = $state('');
  let editKeyInput = $state('');
  let keyError = $state('');
  let videoRemove = $state(false);

  let submitting = $state(false);
  let deleting = $state(false);
  let formError = $state('');
  let fieldErrors = $state<Record<string, string>>({});

  // --- 投稿完了・アップロード ---
  // svelte-ignore state_referenced_locally
  let workId = $state(id);
  let createdKey = $state('');
  let createdCode = $state('');
  let copiedField = $state<'code' | 'key' | ''>('');
  let copyFailedField = $state<'code' | 'key' | ''>('');

  let uploader: VideoUploader | null = null;
  let uploadSnap = $state<UploadSnapshot>({ phase: 'idle', sentBytes: 0, totalBytes: 0, error: '' });
  let pendingResume = $state<StoredUpload | null>(null);
  let resumeError = $state('');
  let processingResult = $state<'processing' | 'ready' | 'failed' | 'timeout'>('processing');
  let processedWork = $state<WorkRecord | null>(null);
  let destroyed = false;

  $effect(() => {
    return () => {
      destroyed = true;
    };
  });

  function onReady(ev: EventRecord) {
    event = ev;
    if (!isEdit) {
      step = 'form';
      return;
    }
    const stored = getEditKey(id);
    if (stored) {
      editKey = stored;
      void loadWork();
    } else {
      step = 'needkey';
    }
  }

  async function loadWork() {
    step = 'loading';
    loadError = '';
    try {
      const w = await getWork(slug, id);
      work = w;
      description = w.description;
      videoUrl = w.video_url;
      demoUrl = w.demo_url;
      githubUrl = w.github_url;
      tagsText = w.tags.join(', ');
      links = w.links.map((l) => ({ title: l.title, url: l.url }));
      existingImages = w.images.map((name) => ({ name, removed: false }));
      pendingResume = getUpload(id);
      step = 'form';
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        loadError = '作品が見つかりませんでした。';
      } else {
        loadError = errMsg(err);
      }
      step = 'form';
    }
  }

  function submitEditKey(e: SubmitEvent) {
    e.preventDefault();
    const v = editKeyInput.trim();
    if (!v) {
      keyError = '編集キーを入力してください';
      return;
    }
    keyError = '';
    editKey = v;
    void loadWork();
  }

  // --- 画像 ---

  const activeImageCount = $derived(
    existingImages.filter((i) => !i.removed).length + newImages.length,
  );

  async function addImages(files: FileList | File[]) {
    imageError = '';
    imageBusy = true;
    for (const f of Array.from(files)) {
      if (activeImageCount >= 10) {
        imageError = '画像は10枚までです';
        break;
      }
      if (!f.type.startsWith('image/')) {
        imageError = '画像ファイルを選んでください';
        continue;
      }
      try {
        const processed = await processImage(f, imageSeq++);
        newImages.push({ file: processed, url: URL.createObjectURL(processed) });
      } catch (err) {
        imageError = errMsg(err);
      }
    }
    imageBusy = false;
  }

  function onImagePick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    if (input.files && input.files.length > 0) void addImages(input.files);
    input.value = '';
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragOver = false;
    if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
      void addImages(e.dataTransfer.files);
    }
  }

  function removeNewImage(i: number) {
    URL.revokeObjectURL(newImages[i].url);
    newImages.splice(i, 1);
  }

  // --- 動画ファイル選択 ---

  function onVideoPick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const f = input.files?.[0] ?? null;
    fieldErrors['video'] = '';
    if (f) {
      const ext = fileExt(f.name);
      if (!VIDEO_EXTS.includes(ext)) {
        fieldErrors['video'] = `対応していない形式です（${VIDEO_EXTS.join(' / ')}）`;
        input.value = '';
        return;
      }
      if (event && f.size > event.max_video_bytes) {
        fieldErrors['video'] = `動画は最大 ${formatGB(event.max_video_bytes)} までです`;
        input.value = '';
        return;
      }
      if (f.size <= 0) {
        fieldErrors['video'] = 'ファイルが空です';
        input.value = '';
        return;
      }
    }
    videoFile = f;
  }

  // --- バリデーション・送信 ---

  function parseTags(): string[] | null {
    const tags = [...new Set(tagsText.split(/[,、]/).map((t) => t.trim()).filter(Boolean))];
    if (tags.length > 10) {
      fieldErrors['tags'] = 'タグは10個までです';
      return null;
    }
    for (const t of tags) {
      if (t.length > 20) {
        fieldErrors['tags'] = 'タグは1つ20文字までです';
        return null;
      }
    }
    return tags;
  }

  function addLink() {
    if (links.length >= 5) return;
    links.push({ title: '', url: '' });
  }

  function removeLink(i: number) {
    links.splice(i, 1);
  }

  function validate(): FormData | null {
    fieldErrors = {};
    formError = '';

    const desc = description.trim();
    if (!desc) fieldErrors['description'] = '説明、アピールを入力してください';
    else if (desc.length > 10000) fieldErrors['description'] = '説明、アピールは10,000文字以内です';

    const urlOk = (u: string) => !u || /^https?:\/\//.test(u);
    const vUrl = videoUrl.trim();
    if (!urlOk(vUrl)) fieldErrors['video_url'] = 'http:// または https:// のURLを入力してください';
    const dUrl = demoUrl.trim();
    if (!urlOk(dUrl)) fieldErrors['demo_url'] = 'http:// または https:// のURLを入力してください';
    const gUrl = githubUrl.trim();
    if (!urlOk(gUrl)) fieldErrors['github_url'] = 'http:// または https:// のURLを入力してください';

    const tags = parseTags();

    if (activeImageCount > 10) fieldErrors['images'] = '画像は10枚までです';

    const cleanedLinks: WorkLink[] = [];
    for (const l of links) {
      const title = l.title.trim();
      const url = l.url.trim();
      if (!title && !url) continue;
      if (!title || !url) {
        fieldErrors['links'] = 'リンクはタイトルとURLの両方を入力してください';
        break;
      }
      if (title.length > 30) {
        fieldErrors['links'] = 'リンクのタイトルは1〜30文字で入力してください';
        break;
      }
      if (!urlOk(url)) {
        fieldErrors['links'] = 'リンクは http:// または https:// のURLを入力してください';
        break;
      }
      cleanedLinks.push({ title, url });
    }
    if (cleanedLinks.length > 5) fieldErrors['links'] = 'リンクは5件までです';

    // 中身条件（SPEC §8.2）：画像・動画のどちらかは必ず要る。
    // 動画ファイルは保存後にアップロードするので、ここでは video_pending として申告する。
    const videoPending = !!videoFile;
    const keepsVideo =
      isEdit && !videoRemove && !!work && work.video_status !== 'none';
    if (!videoPending && !keepsVideo && activeImageCount === 0 && !vUrl) {
      fieldErrors['content'] = '画像か動画のどちらかを必ず入れてください';
    }

    if (tags === null || Object.keys(fieldErrors).some((k) => fieldErrors[k])) {
      // 項目別エラーが出ているときは下部バナーを重ねない
      formError = '';
      return null;
    }

    const fd = new FormData();
    fd.set('description', desc);
    fd.set('video_url', vUrl);
    fd.set('demo_url', dUrl);
    fd.set('github_url', gUrl);
    fd.set('tags', JSON.stringify(tags));
    fd.set('links', JSON.stringify(cleanedLinks));
    if (videoPending) fd.set('video_pending', '1');
    for (const img of newImages) fd.append('images', img.file, img.file.name);

    if (isEdit) {
      const remove = existingImages.filter((i) => i.removed).map((i) => i.name);
      if (remove.length > 0) fd.set('images_remove', JSON.stringify(remove));
      if (videoRemove) fd.set('video_remove', '1');
    }
    return fd;
  }

  /** 最初の項目エラーまでスクロールする（描画後に呼ぶ） */
  async function focusFirstError() {
    await tick();
    document
      .querySelector('.main-form .error-text')
      ?.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }

  function handleSubmitError(err: unknown) {
    if (err instanceof ApiError) {
      if (err.status === 422 && Object.keys(err.fields).length > 0) {
        for (const [k, v] of Object.entries(err.fields)) fieldErrors[k] = String(v);
        formError = '';
        void focusFirstError();
        return;
      }
      if (err.status === 403) {
        keyError = '編集キーが違います';
        step = 'needkey';
        return;
      }
      if (err.status === 423) {
        formError = '受付は終了しました';
        return;
      }
    }
    formError = errMsg(err);
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (submitting) return;
    const fd = validate();
    if (!fd) {
      void focusFirstError();
      return;
    }
    submitting = true;
    try {
      if (isEdit) {
        const r = await updateWork(slug, id, editKey, fd);
        work = r.work;
        setEditKey(id, editKey);
        if (videoFile) {
          void startUpload(videoFile, null);
        } else {
          navigate(`/e/${slug}/w/${id}`);
        }
      } else {
        const r = await createWork(slug, fd);
        workId = r.work.id;
        createdKey = r.edit_key;
        createdCode = r.work_code;
        editKey = r.edit_key;
        setEditKey(workId, r.edit_key);
        setWorkCode(workId, r.work_code);
        step = 'editkey';
        window.scrollTo(0, 0);
      }
    } catch (err) {
      handleSubmitError(err);
    }
    submitting = false;
  }

  async function onDelete() {
    if (!confirm('この作品を削除しますか？画像・動画・いいねもすべて消えます。この操作は取り消せません。')) {
      return;
    }
    deleting = true;
    formError = '';
    try {
      await deleteWork(slug, id, editKey);
      removeEditKey(id);
      removeWorkCode(id);
      removeUpload(id);
      navigate(`/e/${slug}`);
    } catch (err) {
      handleSubmitError(err);
      deleting = false;
    }
  }

  async function copy(field: 'code' | 'key') {
    copyFailedField = '';
    if (await copyText(field === 'code' ? createdCode : createdKey)) {
      copiedField = field;
      setTimeout(() => (copiedField = ''), 2000);
    } else {
      copyFailedField = field;
    }
  }

  // --- 動画アップロード ---

  async function startUpload(file: File, resumeMeta: StoredUpload | null) {
    step = 'upload';
    window.scrollTo(0, 0);
    uploader = new VideoUploader({
      slug,
      workId,
      editKey,
      file,
      onChange: (s) => {
        uploadSnap = s;
      },
    });
    try {
      const result = resumeMeta ? await uploader.start(resumeMeta) : await uploader.start();
      if (result === null) {
        // キャンセル → onCancel ハンドラ側で作品ページへ移動する
        return;
      }
      startPolling();
    } catch {
      // uploadSnap.phase === 'failed'（UploadProgress がエラーと再試行を表示）
    }
  }

  function retryUpload() {
    if (!videoFile && !resumeFile) return;
    const f = videoFile ?? resumeFile;
    if (!f) return;
    void startUpload(f, getUpload(workId));
  }

  function startPolling() {
    step = 'processing';
    processingResult = 'processing';
    pollVideoStatus(slug, workId, {
      onUpdate: (w) => {
        processedWork = w;
      },
      shouldStop: () => destroyed,
    })
      .then((w) => {
        processedWork = w;
        processingResult = w.video_status === 'ready' ? 'ready' : 'failed';
      })
      .catch((err: unknown) => {
        if (err instanceof Error && err.message === 'stopped') return;
        processingResult = 'timeout';
      });
  }

  // --- リロード後の再開 ---

  let resumeFile = $state<File | null>(null);

  function onResumePick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const f = input.files?.[0] ?? null;
    resumeError = '';
    if (!f || !pendingResume) return;
    if (f.size !== pendingResume.size || fileExt(f.name) !== pendingResume.ext) {
      resumeError = '前回と同じファイルを選んでください（サイズまたは形式が一致しません）';
      input.value = '';
      return;
    }
    resumeFile = f;
    void startUpload(f, pendingResume);
  }

  async function abandonResume() {
    if (!pendingResume) return;
    try {
      await videoCancel(slug, workId, editKey, pendingResume.uploadId);
    } catch {
      // サーバー側は掃除 cron が回収する
    }
    removeUpload(workId);
    pendingResume = null;
  }

  function beforeUnload(e: BeforeUnloadEvent) {
    if (
      step === 'upload' &&
      (uploadSnap.phase === 'initializing' ||
        uploadSnap.phase === 'uploading' ||
        uploadSnap.phase === 'paused' ||
        uploadSnap.phase === 'completing')
    ) {
      e.preventDefault();
    }
  }
</script>

<svelte:window onbeforeunload={beforeUnload} />

<EventGuard {slug} onReady={onReady}>
  {#snippet children({ event: ev })}
    <p class="back">
      <a href={isEdit ? `/e/${slug}/w/${id}` : `/e/${slug}`} onclick={onLinkClick}>← 戻る</a>
    </p>

    {#if !ev.submissions_open && step === 'form'}
      <div class="notice-box">受付は終了しました。投稿・編集はできません。</div>
    {/if}

    {#if step === 'loading'}
      <p class="status-msg">読み込み中…</p>
    {:else if step === 'needkey'}
      <div class="stage-center">
        <form class="card key-form" onsubmit={submitEditKey}>
          <h1>編集キーの入力</h1>
          <p class="hint">
            この作品を編集するには、投稿時に表示された編集キーが必要です。忘れた場合は先生に聞いてください。
          </p>
          <input
            type="text"
            bind:value={editKeyInput}
            placeholder="編集キー"
            autocomplete="off"
            autocapitalize="off"
            spellcheck="false"
          />
          {#if keyError}<p class="error-text">{keyError}</p>{/if}
          <button type="submit" class="btn btn-primary">続ける</button>
        </form>
      </div>
    {:else if step === 'form'}
      {#if loadError}
        <div class="error-box"><p>{loadError}</p></div>
      {:else}
        <h1>{isEdit ? '作品を編集' : '作品を投稿'}</h1>

        {#if isEdit && pendingResume}
          <div class="notice-box resume-box">
            <p><strong>前回の動画アップロードが途中で終わっています。</strong></p>
            <p>同じ動画ファイルをもう一度選ぶと、続きからアップロードします。</p>
            <label class="btn btn-sm">
              ファイルを選んで再開
              <input type="file" accept="video/*,.mov,.mkv,.avi,.m4v" hidden onchange={onResumePick} />
            </label>
            <button type="button" class="btn btn-sm btn-danger" onclick={abandonResume}>
              アップロードをやめる
            </button>
            {#if resumeError}<p class="error-text">{resumeError}</p>{/if}
          </div>
        {/if}

        <form class="main-form" onsubmit={submit} novalidate>
          <div class="notice-box privacy-note">
            個人情報は載せないでください。
          </div>

          <div class="field">
            <div class="label-row">
              <label for="f-desc">説明、アピール（Markdown可） <span class="req">必須</span></label>
              <button type="button" class="btn btn-sm" onclick={() => (showPreview = !showPreview)}>
                {showPreview ? '編集に戻る' : 'プレビュー'}
              </button>
            </div>
            {#if showPreview}
              <div class="card preview markdown-body">
                {#if description.trim()}
                  <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                  {@html renderMarkdown(description)}
                {:else}
                  <p class="hint">（まだ何も書かれていません）</p>
                {/if}
              </div>
            {:else}
              <textarea id="f-desc" rows="8" bind:value={description}></textarea>
            {/if}
            {#if fieldErrors['description']}<p class="error-text">{fieldErrors['description']}</p>{/if}
          </div>

          {#if fieldErrors['content']}
            <p class="error-text">{fieldErrors['content']}</p>
          {/if}

          <div class="field">
            <span class="label">画像（最大10枚）</span>
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="dropzone"
              class:drag={dragOver}
              ondragover={(e) => {
                e.preventDefault();
                dragOver = true;
              }}
              ondragleave={() => (dragOver = false)}
              ondrop={onDrop}
            >
              <p>ここに画像をドロップ、または</p>
              <label class="btn btn-sm">
                画像を選ぶ
                <input type="file" accept={IMAGE_ACCEPT} multiple hidden onchange={onImagePick} />
              </label>
              {#if imageBusy}<p class="hint">画像を処理中…</p>{/if}
            </div>
            {#if imageError}<p class="error-text">{imageError}</p>{/if}
            {#if fieldErrors['images']}<p class="error-text">{fieldErrors['images']}</p>{/if}

            {#if existingImages.length > 0 || newImages.length > 0}
              <div class="thumbs">
                {#each existingImages as img (img.name)}
                  {#if work}
                    <div class="thumb-item" class:removed={img.removed}>
                      <img src={fileUrl(work, img.name, true)} alt="" />
                      <button
                        type="button"
                        class="thumb-x"
                        onclick={() => (img.removed = !img.removed)}
                        aria-label={img.removed ? '削除を取り消す' : 'この画像を削除'}
                      >
                        {img.removed ? '戻す' : '×'}
                      </button>
                    </div>
                  {/if}
                {/each}
                {#each newImages as img, i (img.url)}
                  <div class="thumb-item">
                    <img src={img.url} alt="" />
                    <button
                      type="button"
                      class="thumb-x"
                      onclick={() => removeNewImage(i)}
                      aria-label="この画像を取り消す"
                    >
                      ×
                    </button>
                  </div>
                {/each}
              </div>
              <p class="hint">×で削除できます。並び順は選んだ順です。</p>
            {/if}
          </div>

          <div class="field">
            <span class="label">動画</span>
            {#if isEdit && work && work.video_status !== 'none' && !videoRemove}
              <div class="current-video">
                <p class="hint">
                  現在の動画：{work.video_status === 'ready'
                    ? 'あり（再生可能）'
                    : work.video_status === 'failed'
                      ? '変換に失敗'
                      : '変換中'}
                </p>
                <button type="button" class="btn btn-sm btn-danger" onclick={() => (videoRemove = true)}>
                  動画を削除する
                </button>
              </div>
            {:else if videoRemove}
              <div class="notice-box">
                保存すると動画を削除します。
                <button type="button" class="btn btn-sm" onclick={() => (videoRemove = false)}>
                  取り消す
                </button>
              </div>
            {/if}
            <div class="video-pick">
              <label class="btn btn-sm" class:disabled={!!pendingResume}>
                動画を選ぶ
                <input
                  type="file"
                  accept="video/*,.mov,.mkv,.avi,.m4v"
                  hidden
                  onchange={onVideoPick}
                  disabled={!!pendingResume}
                />
              </label>
            </div>
            {#if event}
              <p class="hint">
                最大 {formatGB(event.max_video_bytes)}。{isEdit
                  ? '保存後にアップロードが始まります。'
                  : '投稿の保存が終わってからアップロードが始まります。'}
              </p>
            {/if}
            {#if videoFile}<p class="hint">選択中：{videoFile.name}</p>{/if}
            {#if fieldErrors['video']}<p class="error-text">{fieldErrors['video']}</p>{/if}
          </div>

          <div class="field links-block">
            <span class="label">リンク</span>
            <label for="f-demo">公開URL</label>
            <input
              id="f-demo"
              type="url"
              bind:value={demoUrl}
              placeholder="https://..."
              inputmode="url"
            />
            {#if fieldErrors['demo_url']}<p class="error-text">{fieldErrors['demo_url']}</p>{/if}

            <label for="f-github">GitHub</label>
            <input
              id="f-github"
              type="url"
              bind:value={githubUrl}
              placeholder="https://github.com/..."
              inputmode="url"
            />
            {#if fieldErrors['github_url']}<p class="error-text">{fieldErrors['github_url']}</p>{/if}

            <label for="f-vurl">動画URL</label>
            <input
              id="f-vurl"
              type="url"
              bind:value={videoUrl}
              placeholder="https://www.youtube.com/watch?v=..."
              inputmode="url"
            />
            {#if fieldErrors['video_url']}<p class="error-text">{fieldErrors['video_url']}</p>{/if}

            <span class="label sub-label">その他のリンク</span>
            {#each links as link, i (i)}
              <div class="link-row">
                <input
                  type="text"
                  bind:value={link.title}
                  placeholder="タイトル"
                  maxlength="30"
                  aria-label={`リンク ${i + 1} のタイトル`}
                />
                <input
                  type="url"
                  bind:value={link.url}
                  placeholder="https://..."
                  inputmode="url"
                  aria-label={`リンク ${i + 1} のURL`}
                />
                <button type="button" class="btn btn-sm" onclick={() => removeLink(i)} aria-label="このリンクを削除">
                  ×
                </button>
              </div>
            {/each}
            <button
              type="button"
              class="btn btn-sm add-link"
              onclick={addLink}
              disabled={links.length >= 5}
            >
              ＋ リンクを追加
            </button>
            {#if links.length >= 5}<p class="hint">リンクは5件までです</p>{/if}
            {#if fieldErrors['links']}<p class="error-text">{fieldErrors['links']}</p>{/if}
          </div>

          <div class="field">
            <label for="f-tags">タグ（カンマ区切り、最大10個）</label>
            <input id="f-tags" type="text" bind:value={tagsText} placeholder="例: くじ引き, Unity" />
            {#if fieldErrors['tags']}<p class="error-text">{fieldErrors['tags']}</p>{/if}
          </div>

          {#if formError}<div class="error-box">{formError}</div>{/if}

          <div class="submit-row">
            <button
              type="submit"
              class="btn btn-primary submit-btn"
              disabled={submitting || imageBusy || !ev.submissions_open}
            >
              {submitting ? '保存中…' : isEdit ? '保存する' : '投稿する'}
            </button>
            {#if isEdit}
              <button type="button" class="btn btn-danger" onclick={onDelete} disabled={deleting}>
                {deleting ? '削除中…' : '削除する'}
              </button>
            {/if}
          </div>
        </form>
      {/if}
    {:else if step === 'editkey'}
      <div class="stage-center">
      <div class="card done-card">
        <h1>投稿しました</h1>

        <p>
          これは<strong>作品コード</strong>です。あとでアンケートに書いてもらいます。控えておいてください。
        </p>
        <div class="key-display">
          <code class="work-code">{createdCode}</code>
          <button type="button" class="btn btn-sm" onclick={() => copy('code')}>
            {copiedField === 'code' ? 'コピーしました' : 'コピー'}
          </button>
        </div>
        {#if copyFailedField === 'code'}
          <p class="error-text">コピーできませんでした。手動で控えてください</p>
        {/if}

        <p class="second-key">
          これは<strong>編集キー</strong>です。あとで作品を編集・削除するときに必要です。
        </p>
        <div class="key-display">
          <code>{createdKey}</code>
          <button type="button" class="btn btn-sm" onclick={() => copy('key')}>
            {copiedField === 'key' ? 'コピーしました' : 'コピー'}
          </button>
        </div>
        {#if copyFailedField === 'key'}
          <p class="error-text">コピーできませんでした。手動で控えてください</p>
        {/if}

        <p class="warn-text">
          この画面を閉じると再表示できません。先生に聞けば再発行できます。
          （この端末には自動保存され、「編集キーを管理」画面から見返せます）
        </p>
        {#if videoFile}
          <button type="button" class="btn btn-primary" onclick={() => videoFile && startUpload(videoFile, null)}>
            動画のアップロードへ進む
          </button>
        {:else}
          <a class="btn btn-primary" href={`/e/${slug}/w/${workId}`} onclick={onLinkClick}>
            作品ページを見る
          </a>
        {/if}
      </div>
      </div>
    {:else if step === 'upload'}
      <h1>動画のアップロード</h1>
      <UploadProgress
        snapshot={uploadSnap}
        onPause={() => uploader?.pause()}
        onResume={() => uploader?.resume()}
        onCancel={async () => {
          await uploader?.cancel();
          navigate(`/e/${slug}/w/${workId}`);
        }}
        onRetry={retryUpload}
      />
    {:else if step === 'processing'}
      <div class="stage-center">
      <div class="card done-card">
        {#if processingResult === 'processing'}
          <h1>変換中</h1>
          <p>
            動画をブラウザで再生できる形式に変換しています（数分かかります）。
            このページを閉じても変換は続きます。
          </p>
        {:else if processingResult === 'ready'}
          <h1>完了しました</h1>
          <p>動画の変換が終わり、再生できるようになりました。</p>
        {:else if processingResult === 'failed'}
          <h1>変換に失敗しました</h1>
          <p class="error-text">
            {processedWork?.video_error ||
              '動画を変換できませんでした。別の形式で書き出すか、YouTube の限定公開URLをお使いください。'}
          </p>
        {:else}
          <h1>変換に時間がかかっています</h1>
          <p>しばらくしてから作品ページを確認してください。</p>
        {/if}
        <a class="btn btn-primary" href={`/e/${slug}/w/${workId}`} onclick={onLinkClick}>
          作品ページを見る
        </a>
      </div>
      </div>
    {/if}
  {/snippet}
</EventGuard>

<style>
  form.main-form {
    max-width: 720px;
  }

  .back {
    margin: 0 0 24px;
    font-size: 0.9rem;
  }

  h1 {
    font-size: 1.5rem;
    margin: 0 0 32px;
  }

  .key-form {
    width: 100%;
    max-width: 480px;
    padding: 24px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin: 0 auto;
  }

  .key-form h1 {
    margin: 0;
  }

  .req {
    color: var(--danger);
    font-size: 0.75rem;
    font-weight: 700;
    margin-left: 4px;
  }

  .label-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 6px;
  }

  .label-row label {
    font-weight: 700;
    font-size: 0.95rem;
  }

  .preview {
    padding: 12px 16px;
    min-height: 120px;
  }

  .dropzone {
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    padding: 24px 16px;
    text-align: center;
    color: var(--muted);
  }

  .dropzone p {
    margin: 0 0 8px;
  }

  .dropzone.drag {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 6%, var(--surface));
  }

  .thumbs {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(90px, 1fr));
    gap: 8px;
    margin-top: 10px;
  }

  .thumb-item {
    position: relative;
    aspect-ratio: 1;
    border: var(--hairline);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .thumb-item img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .thumb-item.removed img {
    opacity: 0.3;
    filter: grayscale(1);
  }

  .thumb-x {
    position: absolute;
    top: 4px;
    right: 4px;
    background: rgba(0, 0, 0, 0.65);
    color: #fff;
    border: none;
    border-radius: var(--radius);
    min-width: 26px;
    height: 26px;
    padding: 0 6px;
    cursor: pointer;
    font-size: 0.8rem;
  }

  .btn.disabled {
    opacity: 0.45;
    pointer-events: none;
  }

  .current-video {
    margin-bottom: 8px;
  }

  .links-block > label,
  .links-block > .sub-label {
    margin-top: 16px;
  }

  .link-row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    margin-bottom: 8px;
  }

  .link-row input:first-child {
    flex: 1 1 120px;
  }

  .link-row input:nth-child(2) {
    flex: 2 1 180px;
  }

  .add-link {
    margin-top: 4px;
  }

  .privacy-note {
    margin: 0 0 32px;
  }

  .submit-row {
    display: flex;
    gap: 16px;
    align-items: center;
    margin: 32px 0 0;
  }

  .submit-btn {
    flex: 1;
    max-width: 320px;
  }

  .done-card {
    width: 100%;
    max-width: 560px;
    margin: 0 auto;
    padding: 28px 24px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .done-card h1 {
    margin: 0;
  }

  .key-display {
    display: flex;
    align-items: center;
    gap: 12px;
    background: var(--fill);
    border-radius: var(--radius);
    padding: 16px 20px;
  }

  .key-display code {
    font-size: 1.05rem;
    font-weight: 700;
    letter-spacing: 0.03em;
    word-break: break-all;
    flex: 1;
  }

  /* 作品コードは書き写す前提なので、編集キーより大きく字間を空ける */
  .key-display code.work-code {
    font-size: 1.5rem;
    letter-spacing: 0.12em;
  }

  .second-key {
    margin-top: 8px;
  }

  .warn-text {
    color: var(--text);
    font-size: 0.88rem;
    margin: 0;
  }

  .resume-box .btn {
    margin-right: 8px;
    margin-top: 4px;
  }
</style>
