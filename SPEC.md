# 学内作品共有サイト（学内版ProtoPedia）仕様書

- 版：v1.0（2026-09-11）
- 対象読者：実装を担当するAIコーディングエージェント／開発者
- 本書の位置づけ：v1（初回の校内ハッカソン）で作るものを定義する。「v2以降」と書いた項目は作らない。

---

## 1. 目的とスコープ

校内ハッカソン（文化祭の模擬店で使うくじ引きアプリ開発）で、学生が制作物の画像・動画・アピール文を投稿し、他の学生が閲覧・「いいね」できるサイトを作る。

### 1.1 満たすべき要件

| # | 要件 | 補足 |
|---|---|---|
| R1 | 学生が作品（タイトル・アピール文・画像・動画・デモURL）を投稿・編集・削除できる | アカウント登録なし |
| R2 | 他の学生が作品一覧・詳細を閲覧できる | |
| R3 | **学生同士では作者が分からない**（匿名） | 先生（管理者）は作者を把握できる |
| R4 | 校内・校外どちらからでも投稿・閲覧できる | Cloudflare Tunnel 経由で公開 |
| R5 | 閲覧・投稿はイベントごとの「合言葉」で制限する | |
| R6 | 動画は最大 **2GB** までアップロードできる | Cloudflare の 100MB/リクエスト制限を分割アップロードで回避 |
| R7 | 動画はブラウザで確実に再生できる形式に変換する | H.264/AAC mp4、長辺1080p上限 |
| R8 | 「いいね」ができる（端末単位で1作品1回） | コメントは v2 |
| R9 | 無料で運用する | 有料要素はドメイン代のみ |
| R10 | 開発環境は `flake.nix` で再現できる | |

### 1.2 作らないもの（v2以降）

- コメント機能
- 審査・採点・ランキング（いいね数でのソートのみ v1 に含む）
- 学生アカウント／ログイン
- ファイル配信の合言葉チェック（§9.3 参照）
- 外部ストレージ（S3/R2）。ストレージはローカルディスク

---

## 2. 決定事項一覧

| 項目 | 決定 | 理由 |
|---|---|---|
| バックエンド | PocketBase を **Go ライブラリとして組み込んだ単一バイナリ**（`cmd/server`） | 分割アップロードのバイナリI/O、ffmpeg 起動、マイグレーションが Go だと素直に書ける。`nix build` で1バイナリになる |
| フロントエンド | Vite + Svelte 5 + TypeScript。ビルド成果物を PocketBase の `pb_public` から配信 | 1オリジンで済み CORS 不要。実装者が慣れていれば Vanilla TS / React に差し替えてよい（API仕様は不変） |
| DB／ストレージ | PocketBase 内蔵 SQLite ＋ ローカルファイル | |
| 公開経路 | 学内サーバー → `cloudflared`（Tunnel） → Cloudflare → インターネット | ポート開放・固定IP不要 |
| アクセス制御 | イベントごとの合言葉（HTTPヘッダ `X-Event-Key`） | |
| 編集権限 | 投稿時に発行する編集キー（HTTPヘッダ `X-Edit-Key`） | |
| 匿名性 | 作者情報・編集キーは `work_secrets` コレクションに隔離し、管理者以外読めない | |
| 動画上限 | 2GB／本、1作品につき1本 | |
| チャンクサイズ | 20MB | Cloudflare 100MB 制限と PocketBase のデフォルトボディ上限（32MB）の両方に余裕 |
| 動画変換 | ffmpeg で H.264/AAC mp4、長辺1080p上限、faststart | iPhone の HEVC 対策 |
| いいね | `X-Device-Id`（端末が生成するUUID）単位で重複防止 | なりすまし可能だが学内用途では許容 |
| 開発環境 | `flake.nix`（devShell＋パッケージ） | |

---

## 3. 全体構成

```
[学生のブラウザ] ──HTTPS──▶ [Cloudflare]
                                 │  Tunnel（学内サーバーからの外向き接続）
                                 ▼
                        [学内サーバー]
                          cloudflared ──▶ works-server (127.0.0.1:8090)
                                           ├─ PocketBase 標準API  /api/collections/...（読み取り専用）
                                           ├─ カスタムAPI         /api/x/...（全ての書き込み）
                                           ├─ 管理画面            /_/
                                           ├─ 静的フロント        /（pb_public）
                                           ├─ cron: 動画変換 (ffmpeg)、掃除
                                           └─ pb_data/
                                               ├─ data.db（SQLite）
                                               ├─ storage/   ← 画像・動画・サムネイル
                                               ├─ uploads/   ← 分割アップロードの一時領域
                                               └─ backups/
```

**原則**
- 読み取り（一覧・詳細・イベント情報）は PocketBase 標準の Records API を使い、APIルールで合言葉を検査する。
- 書き込み（作成・更新・削除・動画・いいね）はすべて `/api/x/` 配下のカスタムルートで行う。標準APIの create/update/delete ルールは全コレクションでロック（superuser のみ）。
- ファイル本体は PocketBase 標準の `/api/files/...` から配信する（§9.3 の割り切りを参照）。

---

## 4. リポジトリ構成

```
.
├── flake.nix
├── flake.lock
├── .envrc                 # direnv 用: use flake
├── justfile               # 開発タスク
├── README.md              # セットアップ手順（本書の §12, §13 を要約）
├── SPEC.md                # 本書
├── server/
│   ├── go.mod
│   ├── cmd/server/main.go # PocketBase 組み込み、ルート・cron・マイグレーション登録
│   ├── internal/
│   │   ├── migrations/    # コレクション定義（Go マイグレーション）
│   │   ├── auth/          # 合言葉・編集キーの検証ヘルパ
│   │   ├── works/         # 作品 CRUD ルート
│   │   ├── upload/        # 分割アップロード
│   │   ├── media/         # ffmpeg / exiftool 呼び出し、cron
│   │   └── likes/         # いいね
│   └── pb_data/           # 開発用（.gitignore）
├── web/
│   ├── package.json
│   ├── vite.config.ts     # dev 時は /api と /_/ を 127.0.0.1:8090 にプロキシ
│   ├── src/
│   │   ├── lib/api.ts     # API クライアント（ヘッダ付与、エラー正規化）
│   │   ├── lib/upload.ts  # 分割アップロードクライアント
│   │   ├── lib/keys.ts    # localStorage（合言葉・編集キー・端末ID）
│   │   ├── lib/image.ts   # クライアント側画像リサイズ・EXIF除去
│   │   └── routes/        # 画面
│   └── dist/              # ビルド成果物（.gitignore）
└── deploy/
    ├── works-server.service     # systemd unit
    ├── cloudflared.service      # systemd unit（cloudflared 同梱の service install でも可）
    ├── cloudflared.config.yml.example
    └── nixos-module.nix         # 任意: サーバーが NixOS の場合
```

---

## 5. 開発環境（flake.nix）

### 5.1 提供するもの

| 出力 | 内容 |
|---|---|
| `devShells.default` | go, gopls, golangci-lint, nodejs_22, ffmpeg, exiftool, cloudflared, sqlite, just, prettier |
| `packages.server` | `buildGoModule` で `server/` をビルド。`makeWrapper` で PATH に ffmpeg・exiftool を通す |
| `packages.web` | `buildNpmPackage` で `web/` をビルドし `$out/`（= dist）を出力 |
| `packages.default` | server + web を束ねたもの。`$out/bin/works-server` と `$out/share/works/pb_public/` |
| `apps.default` | `works-server serve` を起動 |
| `nixosModules.default` | 任意。サーバーが NixOS の場合の systemd サービス定義 |

### 5.2 flake.nix の骨子

```nix
{
  description = "campus works showcase";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        runtimeDeps = with pkgs; [ ffmpeg-headless exiftool ];
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go gopls golangci-lint
            nodejs_22
            ffmpeg exiftool cloudflared sqlite just nodePackages.prettier
          ];
          shellHook = ''
            export WORKS_DEV=1
            echo "works dev shell: 'just dev' で server + web を起動"
          '';
        };

        packages = rec {
          server = pkgs.buildGoModule {
            pname = "works-server";
            version = "0.1.0";
            src = ./server;
            vendorHash = pkgs.lib.fakeHash;   # 初回ビルドで表示される正しい値に置き換える
            subPackages = [ "cmd/server" ];
            nativeBuildInputs = [ pkgs.makeWrapper ];
            postInstall = ''
              wrapProgram $out/bin/server \
                --prefix PATH : ${pkgs.lib.makeBinPath runtimeDeps}
              mv $out/bin/server $out/bin/works-server
            '';
          };

          web = pkgs.buildNpmPackage {
            pname = "works-web";
            version = "0.1.0";
            src = ./web;
            npmDepsHash = pkgs.lib.fakeHash;  # 同上
            installPhase = ''
              mkdir -p $out
              cp -r dist/. $out/
            '';
          };

          default = pkgs.runCommand "works" { } ''
            mkdir -p $out/bin $out/share/works
            ln -s ${server}/bin/works-server $out/bin/works-server
            ln -s ${web} $out/share/works/pb_public
          '';
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/works-server";
        };
      })
    // {
      nixosModules.default = import ./deploy/nixos-module.nix;
    };
}
```

注意点：
- `vendorHash` / `npmDepsHash` は `lib.fakeHash` で一度ビルドし、エラーに表示されるハッシュに置き換える。依存を更新したら再計算する。
- `buildNpmPackage` は `package-lock.json` が必須。
- サーバー実行時は `--publicDir` に `packages.default` の `share/works/pb_public` を渡す（§12）。
- ffmpeg・exiftool は PATH 経由で探すが、環境変数 `WORKS_FFMPEG_BIN` / `WORKS_EXIFTOOL_BIN` で明示もできる（§10.5）。

### 5.3 justfile

```just
default: dev

# server と web の開発サーバーを同時起動
dev:
    just dev-server & just dev-web; wait

dev-server:
    cd server && go run ./cmd/server serve --http=127.0.0.1:8090 --dir=./pb_data --automigrate

dev-web:
    cd web && npm run dev

build:
    nix build .#default

lint:
    cd server && golangci-lint run ./...
    cd web && npm run check

test:
    cd server && go test ./...

# 管理者（superuser）作成（初回のみ）
superuser email password:
    cd server && go run ./cmd/server superuser create {{email}} {{password}} --dir=./pb_data

backup:
    cd server && go run ./cmd/server backup --dir=./pb_data
```

### 5.4 .envrc

```
use flake
```

---
## 6. データモデル

PocketBase のコレクション。**Go マイグレーション**（`server/internal/migrations/`）で定義し、起動時に自動適用する。管理画面で手作業で作らない。

### 6.1 `events`

| フィールド | 型 | 制約・備考 |
|---|---|---|
| id | (自動) | |
| name | text | 必須、1〜80文字 |
| slug | text | 必須、`^[a-z0-9-]{2,40}$`、unique index |
| passphrase | text | 必須、8文字以上。APIルールで参照 |
| max_video_bytes | number | 必須、既定 2147483648（2GB） |
| submissions_open | bool | 既定 true。false の間は作成・更新・削除・動画・いいねを 423 で拒否 |
| description | text | 任意。一覧ページ上部に表示（Markdown） |
| created / updated | autodate | |

### 6.2 `works`

| フィールド | 型 | 制約・備考 |
|---|---|---|
| id | (自動) | |
| event | relation(events) | 必須、maxSelect 1、cascadeDelete true |
| title | text | 必須、1〜60文字 |
| description | text | 任意、最大 10,000文字。Markdown |
| images | file | maxSelect 10、maxSize 10MB、mimeTypes: image/jpeg, image/png, image/webp, image/gif |
| video | file | maxSelect 1、maxSize 2147483648。**クライアントは直接書かない**（分割アップロード完了時と変換完了時にサーバーが差し込む） |
| video_status | select | `none` / `uploading` / `processing` / `ready` / `failed`、既定 `none` |
| video_error | text | 変換失敗時の短い理由（表示用、技術詳細はログへ） |
| thumbnail | file | maxSelect 1、サーバーが生成 |
| video_url | url | 任意。YouTube 等の代替 |
| demo_url | url | 任意。学内サーバー上の作品本体など |
| tags | json | 文字列配列、最大10個、各20文字 |
| like_count | number | 既定 0。`likes` ルートが再集計して更新 |
| active_upload_id | text | 進行中の分割アップロードID。`hidden: true` |
| created / updated | autodate | |

インデックス：`(event, created)`、`(event, like_count)`

### 6.3 `work_secrets`（管理者専用）

| フィールド | 型 | 制約・備考 |
|---|---|---|
| work | relation(works) | 必須、maxSelect 1、cascadeDelete true、**unique index** |
| edit_key | text | 必須。32文字の英数字（`security.RandomString(32)`） |
| author_name | text | 必須、1〜60文字。先生だけが見る |
| author_class | text | 任意、1〜30文字 |
| author_note | text | 任意。連絡先など |
| created | autodate | |

### 6.4 `reactions`（管理者専用）

| フィールド | 型 | 制約・備考 |
|---|---|---|
| work | relation(works) | 必須、cascadeDelete true |
| device_id | text | 必須、UUID v4 形式 |
| created | autodate | |

インデックス：`(work, device_id)` unique

### 6.5 削除の連鎖

`events` 削除 → `works` 削除 → `work_secrets`・`reactions` 削除、ファイルも PocketBase が削除する。管理者が誤って消さないよう、管理画面での events 削除は運用ルールで禁止（§13）。

---

## 7. 認可モデル

### 7.1 ヘッダ

| ヘッダ | 送る側 | 用途 |
|---|---|---|
| `X-Event-Key` | 全リクエスト | 合言葉。`events.passphrase` と一致するイベントに属するデータだけ扱える |
| `X-Edit-Key` | 作品の更新・削除・動画 | 投稿時に発行した編集キー |
| `X-Device-Id` | いいね | 端末が生成し localStorage に保存した UUID v4 |

PocketBase のルール内ではヘッダ名が小文字・アンダースコアに正規化される（`@request.headers.x_event_key`）。

### 7.2 APIルール（標準 Records API）

| コレクション | list / view | create / update / delete |
|---|---|---|
| events | `passphrase = @request.headers.x_event_key` | ロック |
| works | `event.passphrase = @request.headers.x_event_key` | ロック |
| work_secrets | ロック | ロック |
| reactions | ロック | ロック |

「ロック」＝ルールを `nil`（superuser のみ）。カスタムルートは `core.App` を直接使うためルールの影響を受けない。

`events` の view/list は合言葉を知っている人にだけ通るので、レスポンスに `passphrase` が含まれても問題ない（本人が入力した値である）。

### 7.3 カスタムルートの検証手順（共通）

```
requireEvent(e):
  key := e.Request.Header.Get("X-Event-Key")
  if key == "" → 401 {code: "event_key_required"}
  ev := FindFirstRecordByFilter("events", "passphrase = {:k}", k=key)
  if not found → 401 {code: "event_key_invalid"}
  return ev

requireOpen(ev):
  if !ev.GetBool("submissions_open") → 423 {code: "submissions_closed"}

requireEditableWork(e, ev, workId):
  w := FindRecordById("works", workId); if w.event != ev.id → 404
  s := FindFirstRecordByFilter("work_secrets", "work = {:id}")
  if subtle.ConstantTimeCompare(s.edit_key, header X-Edit-Key) != 1 → 403 {code: "edit_key_invalid"}
  return w, s
```

- 編集キーは **平文で保存**する（ハッシュ化しない）。理由：先生が管理画面から学生に再発行・再通知できるようにするため。学内用途で脅威モデルに見合う。
- 合言葉の照合失敗は監査ログ（§9.4）に残す。

### 7.4 レートリミット

PocketBase 設定（Settings → Rate limits）で以下を投入する（マイグレーションで `settings.RateLimits` に書く）。

| ラベル | 上限 |
|---|---|
| `POST /api/x/works` | 10 req / 分 / IP |
| `PUT /api/x/works/` （チャンク） | 120 req / 分 / IP |
| `POST /api/x/works/` （like 含む） | 60 req / 分 / IP |
| `/api/collections/` （読み取り） | 300 req / 分 / IP |
| `POST /api/collections/_superusers/auth-with-password` | 5 req / 分 / IP |

加えて Cloudflare 側にも Rate Limiting ルール（無料枠1本）を `POST` に対して設定する（§12.4）。

---

## 8. カスタム API 仕様（`/api/x/`）

### 8.1 共通

- Content-Type：JSON（作成・更新は multipart/form-data）。
- エラー形式：PocketBase 標準に合わせる。

```json
{ "status": 403, "message": "編集キーが違います", "data": { "code": "edit_key_invalid" } }
```

| status | code | 意味 |
|---|---|---|
| 400 | `bad_request` | パラメータ不正 |
| 401 | `event_key_required` / `event_key_invalid` | 合言葉なし／不一致 |
| 403 | `edit_key_invalid` | 編集キー不一致 |
| 404 | `not_found` | 作品・アップロードなし、または別イベントのもの |
| 409 | `upload_state_conflict` | アップロード状態が不整合（§8.5） |
| 413 | `too_large` | サイズ超過 |
| 422 | `validation` | バリデーション失敗（`data.fields` に項目別メッセージ） |
| 423 | `submissions_closed` | イベント受付終了 |
| 429 | (PocketBase 標準) | レートリミット |

- すべてのルートで `requireEvent` を最初に行う。
- 成功時の作品レスポンスは標準 Records API の `works` レコードと同じ形（クライアントの型を1つにする）。

### 8.2 作品作成 `POST /api/x/works`

multipart/form-data

| フィールド | 必須 | 備考 |
|---|---|---|
| title | ○ | |
| description | | |
| images[] | | 複数可、合計10枚まで |
| video_url | | |
| demo_url | | |
| tags | | JSON 文字列配列 |
| author_name | ○ | `work_secrets` へ |
| author_class | | 同上 |
| author_note | | 同上 |

処理：
1. `requireEvent` → `requireOpen`
2. バリデーション（§6.2 の制約）。URL は `http://` / `https://` のみ。
3. `images` の各ファイルについて **ファイル名を `img_<16文字乱数>.<ext>` に付け替える**（元ファイル名に学生名が入っていることがある）。`ext` は MIME から決める（拡張子を信用しない）。
4. `works` 作成 → 保存。
5. 保存後、画像ファイルを `exiftool -all= -overwrite_original` で処理（§10.4）。
6. `edit_key` を生成し `work_secrets` 作成 → 保存。4〜6 は1トランザクション（`app.RunInTransaction`）。
7. レスポンス `201 { "work": <works record>, "edit_key": "..." }`。**編集キーを返すのはこのときだけ。**

### 8.3 作品更新 `PATCH /api/x/works/:id`

multipart/form-data。ヘッダ `X-Edit-Key` 必須。

| フィールド | 備考 |
|---|---|
| title / description / video_url / demo_url / tags | 送られたものだけ更新 |
| images[] | 追加。既存＋追加が10枚を超えたら 422 |
| images_remove | 削除する既存ファイル名の JSON 配列 |
| video_remove | `"1"` なら動画・サムネイルを削除し `video_status = none` |
| author_name / author_class / author_note | `work_secrets` を更新 |

処理は 8.2 に準じる（`requireEditableWork` を通す）。`video` フィールド本体はこのルートでは受け付けない（送られてきたら 400）。

### 8.4 作品削除 `DELETE /api/x/works/:id`

ヘッダ `X-Edit-Key` 必須。`works` を削除（連鎖で secrets・reactions・ファイルも消える）。進行中の分割アップロード一時ディレクトリがあれば削除。`204`。

### 8.5 動画分割アップロード

固定パラメータ：チャンクサイズ **20,971,520 バイト（20MB）**。サーバーは `chunk_size` がこの値以外なら 400。

#### 8.5.1 開始 `POST /api/x/works/:id/video/init`

```json
{ "filename": "IMG_1234.MOV", "size": 734003200, "chunk_size": 20971520 }
```

処理：
1. `requireEvent` → `requireOpen` → `requireEditableWork`
2. `size <= events.max_video_bytes` でなければ 413。`size > 0`。
3. 拡張子（小文字化）が `mp4, mov, m4v, webm, mkv, avi` のいずれかでなければ 422。
4. 空きディスクが `size * 2.5 + 1GB` 未満なら 507 `{code: "insufficient_storage"}`（`syscall.Statfs`）。
5. 既存の `active_upload_id` があればその一時ディレクトリを削除（再開ではなくやり直し扱い。再開は §8.5.3 で行う）。
6. `upload_id = RandomString(24)`。`pb_data/uploads/<upload_id>/meta.json` を作成：

```json
{ "work_id": "...", "event_id": "...", "filename_ext": ".mov", "size": 734003200,
  "chunk_size": 20971520, "total_chunks": 36, "created": "2026-09-11T10:00:00Z" }
```

7. `works.active_upload_id = upload_id`、`video_status = uploading` で保存。
8. `200 { "upload_id": "...", "chunk_size": 20971520, "total_chunks": 36 }`

#### 8.5.2 チャンク送信 `PUT /api/x/works/:id/video/chunk/:upload_id/:index`

- ボディ：チャンクの生バイト。`Content-Type: application/octet-stream`。
- このルートには `apis.BodyLimit(22 << 20)` を明示的に付ける（デフォルト上限 32MB は超えないが、意図を明示する）。
- 処理：`requireEditableWork` → `meta.json` 読込 → `index` が `0..total_chunks-1` の範囲外なら 400 → 期待サイズ（最後以外は `chunk_size`、最後は `size - chunk_size*(total-1)`）と一致しなければ 400 → `<upload_id>/<index 4桁ゼロ埋め>.part.tmp` に書き、書き終えたら `.part` に rename（原子性）。
- 同じ index の再送は上書き（冪等）。
- `200 { "index": 5, "received": 6 }`（received は受信済み個数）。

#### 8.5.3 状態取得 `GET /api/x/works/:id/video/chunk/:upload_id`

`200 { "received_indexes": [0,1,2,5], "total_chunks": 36, "size": ... }`。クライアントはこれで再開する。

#### 8.5.4 完了 `POST /api/x/works/:id/video/complete/:upload_id`

1. 全チャンク存在・サイズ確認。不足があれば 409 `{code: "upload_state_conflict", data: {missing: [...]}}`。
2. `0000.part` から順に `<upload_id>/assembled<ext>` へ `io.Copy` で結合。合計サイズ一致を確認。
3. `filesystem.NewFileFromPath(assembled)` → `file.Name = "video_<16文字乱数><ext>"` → `works.video` に set、`video_status = processing`、`active_upload_id = ""`、保存。
4. 一時ディレクトリ削除。
5. `200 { "work": <works record> }`。以後クライアントは `video_status` をポーリング（標準 view API、5秒間隔、最大30分）。

#### 8.5.5 中止 `DELETE /api/x/works/:id/video/chunk/:upload_id`

一時ディレクトリ削除、`active_upload_id = ""`、`video_status` を動画があれば `ready` なければ `none` に戻す。`204`。

#### 8.5.6 掃除（cron）

`cleanup_uploads`：15分ごと。`pb_data/uploads/*/meta.json` の `created` が **24時間**より古いディレクトリを削除し、該当 `works.active_upload_id` をクリア、`video_status` を戻す。`meta.json` が無い壊れたディレクトリも削除。

### 8.6 いいね

#### `POST /api/x/works/:id/like`  ヘッダ `X-Device-Id`

トグル。`reactions (work, device_id)` があれば削除、なければ作成。その後 `works.like_count` を `SELECT COUNT(*)` で再集計して保存（差分更新にしない：ズレの自己修復のため）。
`200 { "liked": true, "like_count": 12 }`

#### `GET /api/x/likes`  ヘッダ `X-Device-Id`

クエリ `event=<event_id>`。その端末がいいね済みの `work_id` 配列を返す。一覧・詳細でハート状態を復元する。
`200 { "work_ids": ["...", "..."] }`

`X-Device-Id` は UUID v4 形式でなければ 400。

---
## 9. フロントエンド仕様

### 9.1 画面

パス設計（SPA、`pb_public` の `index.html` にフォールバックさせる）：

| パス | 画面 |
|---|---|
| `/` | イベント選択。localStorage に合言葉があればそのイベント一覧へリダイレクト |
| `/e/:slug` | 作品一覧 |
| `/e/:slug/new` | 投稿 |
| `/e/:slug/w/:id` | 作品詳細 |
| `/e/:slug/w/:id/edit` | 編集（編集キーを localStorage に持っている作品のみリンク表示。持っていなくても手入力で入れる） |

**合言葉ゲート**：`X-Event-Key` 未設定、または API が 401 を返したら、全画面モーダルで合言葉入力を求める。成功したら `localStorage.eventKeys[slug]` に保存。

**作品一覧**
- カードグリッド（モバイル1列、タブレット2列、PC3〜4列）。カード：サムネイル（`thumbnail` → なければ `images[0]` の PocketBase サムネイル `?thumb=600x400` → なければプレースホルダ）、タイトル、いいね数、動画ありバッジ、`video_status` が `processing` なら「変換中」表示。
- 並び替え：新着順（既定）／いいね順。タイトル検索（クライアント側フィルタで十分。標準APIの `filter` は使わず全件取得。1イベント数百件想定）。
- 「投稿する」ボタン。`submissions_open = false` なら非表示にし「受付終了」を表示。

**作品詳細**
- 画像ギャラリー（タップで拡大）。
- 動画：`ready` なら `<video controls playsinline preload="metadata" poster={thumbnail}>`。`processing` なら「変換中（数分かかります）」を表示し10秒間隔で再取得。`failed` なら `video_error` を表示。`video_url` があれば埋め込み（YouTube は `youtube-nocookie.com` の iframe、その他はリンク）。
- アピール文：Markdown → HTML。`marked` ＋ `DOMPurify`。**生HTML禁止、画像記法禁止**、リンクは `http(s)` のみで `rel="noopener nofollow" target="_blank"`。
- デモURL、タグ、いいねボタン（トグル。連打防止に処理中は disabled）。
- 自分の作品（編集キー保持）なら「編集」ボタン。

**投稿／編集フォーム**
- 項目：タイトル、アピール文（textarea＋プレビュー切替）、画像（ドラッグ＆ドロップ・複数・並び替え不要）、動画（ファイル または URL のどちらか）、デモURL、タグ、作者名・クラス（枠で囲い「**この欄は先生だけが見ます。他の学生には表示されません**」と明記）。
- 画像は選択時にクライアントで処理（§9.2）。
- 動画ファイルは保存後に分割アップロード開始（§9.4）。フォーム本体の保存と動画は別ステップにし、本文保存 → 編集キー表示 → 動画アップロードの順。
- 投稿完了時：編集キーを**一度だけ**大きく表示し、コピーボタン、「この画面を閉じると再表示できません。先生に聞けば再発行できます」と注記。同時に `localStorage.editKeys[workId]` に保存。
- 削除は確認ダイアログ付き。

### 9.2 画像のクライアント側処理（`lib/image.ts`）

- 選択された画像を `createImageBitmap` → canvas → `toBlob('image/jpeg', 0.85)`（PNG で透過が必要そうな場合＝アルファチャンネルあり は `image/png`）で再エンコードする。これで EXIF（GPS・端末情報）が落ちる。
- 長辺 2048px に縮小。
- GIF はアニメーションを壊すので再エンコードせずそのまま送る（GIF に EXIF は原則ない）。
- 送信ファイル名は `image.jpg` 等の固定名（サーバー側でさらに乱数名にする）。

### 9.3 ファイルURLについての割り切り

`/api/files/works/<id>/<filename>` は `<img>` `<video>` から読むためヘッダを付けられず、合言葉チェックが効かない。ファイル名は乱数（画像・動画とも `xxx_<16文字乱数>`）で推測不能なので、「一覧を見るには合言葉が要るが、URLを知っていればファイル単体は見える」状態を v1 では許容する。詳細ページのURLを共有すれば合言葉なしで画像は見えないが、画像URLを直接共有すれば見える、という理解を運用側で持っておく。

### 9.4 分割アップロードクライアント（`lib/upload.ts`）

```
state: idle → initializing → uploading(progress) → completing → processing → ready | failed | cancelled
```

- `init` の結果（`upload_id`, `total_chunks`）と `workId` を `localStorage.uploads[workId]` に保存。ファイル自体は保存できないので、リロード後の再開時は「同じファイルをもう一度選んでください」と促し、`size` と拡張子が一致することを確認してから `GET .../chunk/:upload_id` で受信済みを取得し未送信分のみ送る。
- 送信は **並列3**。各チャンクは `File.slice(start, end)` を `fetch` で PUT。失敗（ネットワークエラー、5xx、429）は指数バックオフ（1s, 2s, 4s、最大5回）。4xx（401/403/404/409/413）は即中止しエラー表示。
- 進捗＝受信済みバイト／総バイト。一時停止・再開・中止ボタン。
- ページ離脱時は `beforeunload` で警告。
- 全チャンク成功後 `complete`。409 が返ったら `missing` を再送して再度 `complete`（最大3回）。
- `complete` 成功後は `video_status` をポーリング。

### 9.5 端末ID・鍵の保管（`lib/keys.ts`）

```ts
localStorage["works.deviceId"]        // UUID v4、初回生成
localStorage["works.eventKeys"]       // { [slug]: passphrase }
localStorage["works.editKeys"]        // { [workId]: editKey }
localStorage["works.uploads"]         // { [workId]: { uploadId, size, ext, totalChunks } }
```

「編集キーを管理」画面（設定アイコン）から一覧・削除・手入力できる。

### 9.6 見た目

- モバイルファースト。学生はスマホで投稿・閲覧する前提。
- 派手さより「読みやすい・触りやすい」を優先。ダークモードは `prefers-color-scheme` に追従。
- 文言は日本語。

---

## 10. メディア処理（サーバー）

### 10.1 動画変換 cron `transcode`

- 毎分実行。**同時実行1**（`sync.Mutex` の `TryLock`。取れなければスキップ）。
- `video_status = "processing"` の `works` を `updated` 昇順で1件取得。なければ終了。
- 入力：`pb_data/storage/<works.collectionId>/<record.id>/<video filename>`（ローカルFSストレージ前提）。
- 出力：一時ディレクトリ `pb_data/uploads/_transcode/<record.id>/` に `out.mp4` と `thumb.jpg`。
- 成功時：`filesystem.NewFileFromPath(out.mp4)` を `video_<新乱数>.mp4` として `video` に set、`thumb.jpg` を `thumb_<乱数>.jpg` として `thumbnail` に set、`video_status = ready`、保存。PocketBase が置き換え前の元ファイルを削除することをテストで確認する（削除されない場合は明示的に削除）。
- 失敗時：`video_status = failed`、`video_error` に短い理由（例「動画を変換できませんでした。別の形式で書き出すか、YouTube の限定公開URLをお使いください」）、ffmpeg の stderr 末尾 2KB をログ。元ファイルは残す（管理者が手動対応できるように）。
- タイムアウト：`WORKS_TRANSCODE_TIMEOUT`（既定 90分）。超えたら kill して failed。
- プロセス再起動時、`processing` のまま残った作品は次回 cron で再処理される（べき等）。

### 10.2 ffmpeg コマンド

```
ffmpeg -y -nostdin -hide_banner -loglevel error \
  -i <in> \
  -vf "scale='if(gt(a,1),min(1920,iw),-2)':'if(gt(a,1),-2,min(1920,ih))'" \
  -c:v libx264 -preset medium -crf 23 -pix_fmt yuv420p \
  -c:a aac -b:a 128k -ac 2 \
  -movflags +faststart \
  -threads <WORKS_FFMPEG_THREADS, 既定 2> \
  <out.mp4>
```

- `a` はアスペクト比。横長なら幅1920上限、縦長なら高さ1920上限（=縦動画も1080p相当）。`-2` で偶数に丸める。
- 入力の回転メタデータは ffmpeg が自動適用する（autorotate 既定 on）。
- 音声なしの動画でも失敗しないこと（`-c:a` は音声ストリームが無ければ無視される）。

サムネイル：

```
ffmpeg -y -nostdin -hide_banner -loglevel error \
  -ss 1 -i <in> -frames:v 1 -vf "scale=640:-2" -q:v 4 <thumb.jpg>
```

1秒地点が取れない（1秒未満の動画）場合は `-ss 0` で再試行。

### 10.3 ffprobe による事前検査

変換前に `ffprobe -v error -show_entries format=duration:stream=codec_type -of json` を実行し、映像ストリームが無ければ failed（音声のみファイル等）。`duration` が `WORKS_MAX_VIDEO_SECONDS`（既定 1800 秒＝30分）を超えたら failed（理由「30分以内にしてください」）。

### 10.4 画像の EXIF 除去（サーバー側の保険）

作成・更新ルートで画像保存後、同期的に実行：

```
exiftool -all= -overwrite_original -q <file>...
```

失敗してもリクエストは成功させ、警告ログのみ（クライアント側で再エンコード済みのため）。

### 10.5 環境変数

| 変数 | 既定 | 用途 |
|---|---|---|
| `WORKS_FFMPEG_BIN` | `ffmpeg`（PATH） | |
| `WORKS_FFPROBE_BIN` | `ffprobe` | |
| `WORKS_EXIFTOOL_BIN` | `exiftool` | |
| `WORKS_FFMPEG_THREADS` | `2` | 学内サーバーの他用途を圧迫しない |
| `WORKS_TRANSCODE_TIMEOUT` | `90m` | |
| `WORKS_MAX_VIDEO_SECONDS` | `1800` | |
| `WORKS_UPLOAD_TTL` | `24h` | 未完了アップロードの保持 |

---

## 11. セキュリティ・匿名性チェックリスト

| 項目 | 対応 |
|---|---|
| 作者情報の漏洩 | `work_secrets` は全ルールロック。`works` に作者を示す項目を置かない。ログにも作者名を出さない |
| ファイル名からの推定 | 全ファイルをサーバーで乱数名に付け替え（元名は破棄） |
| 画像メタデータ | クライアント再エンコード＋サーバー exiftool |
| 動画メタデータ | ffmpeg 再エンコードで作成日時・端末情報は落ちる。`-map_metadata -1` を追加してタイトル等も落とす |
| 投稿時刻からの推定 | 一覧の「新着順」で投稿時刻が分かる。学内用途では許容（v1）。気になる場合は表示を日付のみにする |
| Markdown XSS | DOMPurify、生HTML禁止 |
| 管理画面 `/_/` の露出 | superuser パスワードは20文字以上。Cloudflare Access で `/_/*` にメールOTP（管理者1名、無料）を掛ける（§12.4）。API の `/api/collections/_superusers/*` にはレートリミット |
| 合言葉総当たり | レートリミット（§7.4）＋ Cloudflare Rate Limiting。合言葉は8文字以上 |
| 巨大アップロードでのディスク枯渇 | `max_video_bytes`、空き容量チェック、TTL 掃除、1作品1動画 |
| 依存の脆弱性 | `npm audit` / `govulncheck` を CI で実行（任意） |
| 監査ログ | 合言葉不一致・編集キー不一致・作品削除・イベント設定変更を PocketBase の logs に `audit=true` タグ付きで記録（作者名は含めない） |

---

## 12. デプロイ

### 12.1 前提

- 学内サーバー：Linux（x86_64 想定）。Nix（multi-user）インストール済みなら §12.2、無ければ §12.3。
- Cloudflare アカウント（無料）と、Cloudflare にネームサーバーを向けた独自ドメイン1つ。ホスト名は例として `works.example.jp` とする。
- 学内サーバーからインターネットへ HTTPS（443）の外向き接続ができること。

### 12.2 Nix がある場合

```sh
git clone <repo> /opt/works/src
cd /opt/works/src
nix build .#default -o /opt/works/current
sudo useradd -r -s /usr/sbin/nologin works
sudo mkdir -p /var/lib/works/pb_data && sudo chown -R works:works /var/lib/works
sudo cp deploy/works-server.service /etc/systemd/system/
sudo systemctl enable --now works-server
/opt/works/current/bin/works-server superuser create <email> <password> --dir=/var/lib/works/pb_data
```

`deploy/works-server.service`：

```ini
[Unit]
Description=campus works showcase
After=network-online.target
Wants=network-online.target

[Service]
User=works
Group=works
WorkingDirectory=/var/lib/works
ExecStart=/opt/works/current/bin/works-server serve \
  --http=127.0.0.1:8090 \
  --dir=/var/lib/works/pb_data \
  --publicDir=/opt/works/current/share/works/pb_public
Restart=always
RestartSec=5
Environment=WORKS_FFMPEG_THREADS=2
# ハードニング
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/works
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

更新：`nix build .#default -o /opt/works/current && sudo systemctl restart works-server`。マイグレーションは起動時に自動適用（本番でも `migratecmd` の Automigrate は **off**、コード内 `migrations` パッケージの登録分のみ適用）。

サーバーが NixOS なら `deploy/nixos-module.nix` を使い、`services.works.enable = true;` で上記と同等の設定になるようにする（任意）。

### 12.3 Nix が無い場合

開発機で `nix build .#default` し、`result/bin/works-server`（Go 静的バイナリ）と `result/share/works/pb_public` を `scp` する。ffmpeg・exiftool はディストリのパッケージ（`apt install ffmpeg libimage-exiftool-perl`）を入れ、`WORKS_FFMPEG_BIN` 等で必要なら明示。systemd unit は 12.2 と同じ。

### 12.4 Cloudflare

1. **Tunnel 作成**（学内サーバー上）
   ```sh
   cloudflared tunnel login
   cloudflared tunnel create works
   cloudflared tunnel route dns works works.example.jp
   ```
2. `~/.cloudflared/config.yml`（`deploy/cloudflared.config.yml.example` を参照）
   ```yaml
   tunnel: <TUNNEL_ID>
   credentials-file: /etc/cloudflared/<TUNNEL_ID>.json
   ingress:
     - hostname: works.example.jp
       service: http://127.0.0.1:8090
       originRequest:
         noTLSVerify: true
     - service: http_status:404
   ```
3. `sudo cloudflared service install` で systemd 登録（または `deploy/cloudflared.service`）。
4. ダッシュボード設定
   - SSL/TLS：Full（Tunnel なので実質どちらでも可）
   - Security → WAF → Rate limiting rules：`(http.request.method eq "POST" and http.request.uri.path contains "/api/")` を 60 req/分 でブロック（無料枠1本）
   - Zero Trust → Access → Applications：`works.example.jp/_/*` と `works.example.jp/api/collections/_superusers/*` に Self-hosted アプリを作り、ポリシー「メールが `<管理者のメール>` に一致 → Allow（One-time PIN）」。**それ以外のパスには Access を掛けない**（学生には合言葉のみ）
   - Caching：既定でよい。`/api/files/*` はキャッシュされても問題ない（乱数名なので更新時はURLが変わる）
5. 確認：`curl -sI https://works.example.jp/api/health` が 200。

### 12.5 バックアップ

- PocketBase 内蔵バックアップを有効化：Settings → Backups で cron `0 3 * * *`、保持 7 世代（マイグレーションで `settings.Backups` に書いてもよい）。`pb_data/backups/` に zip が作られる。
- 週1回、`pb_data/backups/` を別マシン（先生のPC等）へ `rsync` する運用を README に書く。
- 復元：`works-server` を止め、zip を `pb_data` に展開し直す。

---

## 13. 運用手順（先生向け、README にも転記）

1. 管理画面 `https://works.example.jp/_/` にログイン。
2. `events` に1件作成：`name`「文化祭くじ引きアプリ ハッカソン 2026」、`slug` `fes2026`、`passphrase`（8文字以上、学生に配る）、`max_video_bytes` 2147483648、`submissions_open` true。
3. 学生に `https://works.example.jp/e/fes2026` と合言葉を配る。
4. 作者を確認したいとき：管理画面 → `work_secrets` → `work` で絞り込む。
5. 編集キーを忘れた学生には `work_secrets.edit_key` を伝える（本人確認は先生の判断）。
6. 締切後：`submissions_open` を false にする（閲覧・いいねは継続可。いいねも止めたい場合は仕様上 423 になる）。
7. 不適切な投稿：管理画面から `works` を削除（連鎖で全て消える）。
8. `events` レコードは削除しない（作品が全部消える）。

---

## 14. 実装マイルストーン

| M | 内容 | 完了条件 |
|---|---|---|
| M1 | flake.nix、justfile、`cmd/server` 骨格、マイグレーション、`just dev` で管理画面が開く | `nix develop` → `just dev` が動く。管理画面に4コレクションがある |
| M2 | 合言葉ゲート、作品一覧・詳細（テキストのみ） | 合言葉なしで 401、ありで一覧が見える |
| M3 | 作品作成・更新・削除、画像、編集キー、EXIF 除去 | 投稿→編集キー表示→編集→削除が通る。保存ファイル名が乱数 |
| M4 | 分割アップロード＋変換＋サムネイル＋掃除 cron | 1.5GB の .mov がスマホ回線相当（絞った回線）で完走し、リロード後に再開でき、mp4 になって再生できる |
| M5 | いいね | トグル・件数・端末ごとの状態復元 |
| M6 | デプロイ（systemd、cloudflared、Access、レートリミット、バックアップ） | 校外のスマホから投稿・閲覧できる |
| M7 | 仕上げ：モバイルUI調整、エラー文言、README | 学生2〜3名に触ってもらい詰まらない |

---

## 15. 受け入れテスト（抜粋）

- [ ] `X-Event-Key` 無し／誤りで `GET /api/collections/works/records` が 401 になる（正しい値では 200）
- [ ] `GET /api/collections/work_secrets/records` は正しい合言葉でも 403
- [ ] `works` のレスポンスに `author_*` が一切含まれない
- [ ] 元ファイル名 `山田太郎_くじ.png` で投稿しても保存名・URLに `山田` が含まれない
- [ ] GPS 付き JPEG を投稿後、保存ファイルに EXIF が無い（`exiftool` で確認）
- [ ] 誤った `X-Edit-Key` で PATCH/DELETE が 403
- [ ] 100MB 超の動画が Cloudflare 経由で完走する（チャンクが 100MB 未満であることの確認を兼ねる）
- [ ] チャンク送信中にブラウザを閉じ、再度同じファイルを選ぶと未送信分だけ送られる
- [ ] iPhone の HEVC 画面収録が Windows Chrome で再生できる
- [ ] 縦動画が縦のまま（回転が正しい）、1920 を超える辺がない
- [ ] 音声なし動画、30分超動画、映像なしファイルがそれぞれ想定どおり（成功／failed／failed）
- [ ] `submissions_open = false` で作成・更新・動画・いいねが 423、閲覧は 200
- [ ] 同じ端末で2回いいね → 0 に戻り `like_count` も一致
- [ ] 24時間放置したアップロードが掃除され `video_status` が戻る
- [ ] `nix build .#default` がクリーン環境で成功する
- [ ] `/_/` が Cloudflare Access で保護されている（別ブラウザでログイン画面に到達できない）

---

## 16. 未決事項・注意

- PocketBase の Go API（`core.RequestEvent`、`filesystem.NewFileFromPath`、`Fields.Add`、`hidden` フィールド、`_via_` 逆参照など）は v0.23 系を前提に書いている。実装時に使用バージョンのドキュメントで名称を確認し、差異があれば本書を更新する。
- ファイル置き換え時に PocketBase が旧ファイルを自動削除するかは M4 でテストして確定する。
- 学内サーバーの外部公開（トンネル経由）が学校の方針上問題ないかは、着手前に一度確認する。
- 単一ファイル 2GB の結合・変換で一時的に最大 ~5GB のディスクを使う。`pb_data` は十分な空きのあるパーティションに置く。
