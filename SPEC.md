# 学内作品共有サイト NccHub 仕様書

- 版：v1.0（2026-09-11）
- 対象読者：実装を担当するAIコーディングエージェント／開発者
- 本書の位置づけ：v1（初回の校内ハッカソン）で作るものを定義する。「v2以降」と書いた項目は作らない。

---

## 1. 目的とスコープ

校内ハッカソン（文化祭の模擬店で使うくじ引きアプリ開発）で、学生が制作物の画像・動画・説明を投稿し、他の学生が閲覧・「いいね」できるサイトを作る。

### 1.1 満たすべき要件

| # | 要件 | 補足 |
|---|---|---|
| R1 | 学生が作品（説明、アピール、画像・動画、公開URL・GitHub・リンク）を投稿・編集・削除できる | アカウント登録なし。タイトルは持たない（テーマが1つに決まっているため） |
| R2 | 他の学生が作品一覧・詳細を閲覧できる | |
| R3 | **サイトは作者を特定する情報を一切保持しない** | 氏名・クラス・連絡先の入力欄を置かない。投稿ごとに発行する作品コード（§8.2）を本人に渡し、誰がどれを投稿したかは本サイトの外（アンケート）で集計する |
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
| 匿名性 | 作者情報を保持しない。編集キーと作品コードは `work_secrets` コレクションに隔離し、管理者以外読めない | 保持しなければ漏らしようがない |
| 作者の突合 | 投稿ごとに作品コード（`XXXX-XXXX`）を発行し、投稿完了画面で本人にだけ表示する | 誰がどれを投稿したかは本サイトの外でアンケートを取り、作品コードで突き合わせる |
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
├── README.md              # 開発者ポータル・セットアップ手順
├── SPEC.md                # 本書
├── docs/
│   ├── admin-guide.md     # 先生向け運用マニュアル（本書の §13 を整理）
│   └── deployment.md      # デプロイ・インフラ手順書（本書の §12 を整理）
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
    ├── install.sh               # ワンショットインストーラ（冪等）
    ├── update.sh                # 更新デプロイ（health 失敗時に自動ロールバック）
    ├── backup.sh                # バックアップ（管理 API 経由／--cold）
    ├── works-server.service     # systemd unit のテンプレート
    ├── nixos-module.nix         # 任意: サーバーが NixOS の場合
    └── test/                    # systemd 入り Ubuntu コンテナでの検証一式
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
    cd server && go run ./cmd/server serve --http=127.0.0.1:8090 --dir=./pb_data --automigrate --dev

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

### 5.5 開発用フレーバー

`just dev` は空の `pb_data` でもイベント一覧まで到達できる。
投入は `--dev` かつ待ち受けがループバックのときだけ行う。
本番の systemd と `nix build` 成果物には `--dev` を付けない。
`--dev` は PocketBase 標準フラグである。
`go run` では既定で有効になる。
待ち受けがループバック以外の `--dev` は起動失敗とする。
`WORKS_DEV` は未使用である。
既存の superuser・イベント・作品は上書きしない。

| 項目 | 値 |
|---|---|
| イベント `slug` | `dev` |
| 合言葉 | `dev-aikotoba`（8 文字以上） |
| superuser | `dev@example.com` / `ncc-hub-dev-admin-pass`（20 文字以上。0 件のときだけ作成） |
| サンプル作品 | `dev` に作品が無ければ 2 件。中身条件は `video_url`。いいね数を変えてソート確認用にする |

起動ログに slug・合言葉・作品コード・編集キーを出す。
フロントは `import.meta.env.DEV` のときだけ「開発環境」バーを出す。
トップは `/e/dev` を開き、合言葉を自動入力する。
API の合言葉検査は変えない。
`vite build` 後の `pb_public` には開発用 UI を残さない。

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
| description | text | **必須**、1〜10,000文字。Markdown。先頭行が一覧・詳細の見出しになる（§9.1） |
| images | file | maxSelect 10、maxSize 10MB、mimeTypes: image/jpeg, image/png, image/webp, image/gif |
| video | file | maxSelect 1、maxSize 2147483648。**クライアントは直接書かない**（分割アップロード完了時と変換完了時にサーバーが差し込む） |
| video_status | select | `none` / `uploading` / `processing` / `ready` / `failed`、既定 `none` |
| video_error | text | 変換失敗時の短い理由（表示用、技術詳細はログへ） |
| thumbnail | file | maxSelect 1、サーバーが生成 |
| video_url | url | 任意。YouTube 等。アップロード動画と併用できる |
| demo_url | url | 任意。公開ページ |
| github_url | url | 任意。ソースコード。ホストは github.com に限定しない |
| tags | json | 文字列配列、最大10個、各20文字 |
| links | json | `{title, url}` の配列。最大5件。title は1〜30文字、url は http(s) |
| like_count | number | 既定 0。`likes` ルートが再集計して更新 |
| active_upload_id | text | 進行中の分割アップロードID。`hidden: true` |
| created / updated | autodate | |

インデックス：`(event, created)`、`(event, like_count)`

### 6.3 `work_secrets`（管理者専用）

作品ごとの秘密（編集キーと作品コード）だけを持つ。**氏名・クラス・連絡先など作者を特定する項目は置かない**（§1.1 R3）。

| フィールド | 型 | 制約・備考 |
|---|---|---|
| work | relation(works) | 必須、maxSelect 1、cascadeDelete true、**unique index** |
| edit_key | text | 必須。32文字の英数字（`security.RandomString(32)`） |
| work_code | text | 必須。作品コード。`XXXX-XXXX` 形式（§8.2）。**unique index** |
| created | autodate | |

インデックス：`work` unique、`work_code` unique

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
| description | ○ | 1〜10,000文字 |
| images[] | △ | 複数可、合計10枚まで |
| video_url | △ | アップロード動画と排他ではない |
| video_pending | △ | `"1"` なら、この作成に続けて動画ファイルを分割アップロードする意思表示（§8.5）。作成時点では動画がまだ無いため |
| demo_url | | 公開URL |
| github_url | | ホストは github.com に限定しない |
| tags | | JSON 文字列配列 |
| links | | JSON 文字列。`[{title, url}, ...]`。最大5件 |

△ 印は「`images` / `video_url` / `video_pending=1` のうち少なくとも1つが必要」を意味する（中身の無い投稿を防ぐ）。公開URL・GitHub・その他リンクは中身に数えない。`video_pending` は自己申告なので偽れるが、それで作れるのは空の投稿だけなので v1 では許容する。動画ファイルと `video_url` は排他ではない。

作者を特定する項目は受け取らない。送られてきても無視する。

処理：
1. `requireEvent` → `requireOpen`
2. バリデーション（§6.2 の制約）。URL は `http://` / `https://` のみ。`description` が空なら 422。中身条件（上表 △）を満たさなければ 422（`fields.content`）。
3. `images` の各ファイルについて **ファイル名を `img_<16文字乱数>.<ext>` に付け替える**（元ファイル名に学生名が入っていることがある）。`ext` は MIME から決める（拡張子を信用しない）。
4. `works` 作成 → 保存。
5. 保存後、画像ファイルを `exiftool -all= -overwrite_original` で処理（§10.4）。
6. `edit_key`（32文字）と `work_code`（作品コード）を生成し `work_secrets` 作成 → 保存。4〜6 は1トランザクション（`app.RunInTransaction`）。
7. レスポンス `201 { "work": <works record>, "edit_key": "...", "work_code": "..." }`。**編集キーと作品コードを返すのはこのときだけ。**

**作品コードの生成規則**：`XXXX-XXXX`（英数8文字をハイフンで4文字ずつ区切る）。文字集合は `23456789ABCDEFGHJKMNPQRSTVWXYZ` の30文字。`0/O`・`1/I/L`・`U` を除いてあるのは、紙のアンケートに手で書き写す前提だから。重複したら再生成する（最大5回、それでも衝突したら 500）。

### 8.3 作品更新 `PATCH /api/x/works/:id`

multipart/form-data。ヘッダ `X-Edit-Key` 必須。

| フィールド | 備考 |
|---|---|
| description / video_url / demo_url / github_url / tags / links | 送られたものだけ更新。`description` は送るなら1文字以上（空にはできない） |
| images[] | 追加。既存＋追加が10枚を超えたら 422 |
| images_remove | 削除する既存ファイル名の JSON 配列 |
| video_remove | `"1"` なら動画・サムネイルを削除し `video_status = none` |
| video_pending | `"1"` なら、この更新に続けて動画ファイルを差し替える意思表示 |

処理は 8.2 に準じる（`requireEditableWork` を通す）。`video` フィールド本体はこのルートでは受け付けない（送られてきたら 400）。

更新の結果、画像・動画・`video_url` がすべて無くなり `video_pending` も無い場合は 422（作成時と同じ中身条件）。更新後は実際のレコード状態で判定できるので、ここは厳密に検証する。

作品コードと編集キーは更新できない。レスポンスにも含めない。

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
- カードグリッド（モバイル1列、タブレット2列、PC3〜4列）。カード：サムネイル（`thumbnail` → なければ `images[0]` の PocketBase サムネイル `?thumb=600x400` → なければプレースホルダ）、**見出し**、いいね数、動画ありバッジ、`video_status` が `processing` なら「変換中」表示。
- **見出し**：タイトル欄が無いので `description` の先頭行から作る。Markdown 記法（`#`、`*`、`` ` ``、リンク等）を落としたプレーンテキストの先頭40文字。空行は読み飛ばす。溢れたら CSS で省略する。
- 並び替え：新着順（既定）／いいね順。説明文の全文検索（クライアント側フィルタで十分。標準APIの `filter` は使わず全件取得。1イベント数百件想定）。
- 「投稿する」ボタン。`submissions_open = false` なら非表示にし「受付終了」を表示。

**作品詳細**
- 見出しは置かない（タイトルが無いため）。投稿日と編集ボタンだけをヘッダに出し、説明文がそのまま本文になる。
- 画像ギャラリー（タップで拡大）。
- 動画はファイルと URL を独立して出す。両方あれば上にアップロード動画、下に動画URL。
- アップロード動画：`ready` なら `<video controls playsinline preload="metadata" poster={thumbnail}>`。`processing` / `uploading` なら「変換中（数分かかります）」を表示し10秒間隔で再取得。`failed` なら `video_error` を表示。
- 動画URL：ファイルの状態に関わらず表示する（変換失敗時も含む）。YouTube は `youtube-nocookie.com` の iframe、その他はリンク。
- 説明、アピール：Markdown → HTML。`marked` ＋ `DOMPurify`。**生HTML禁止、画像記法禁止**、リンクは `http(s)` のみで `rel="noopener nofollow" target="_blank"`。
- 公開URL・GitHubはボタン、その他リンクはタイトル付きの箇条書き。タグ、いいねボタン（トグル。連打防止に処理中は disabled）。
- 自分の作品（編集キー保持）なら「編集」ボタン。

**投稿／編集フォーム**
- 項目：**説明、アピール**（textarea＋プレビュー切替、必須）、画像（ドラッグ＆ドロップ・複数・並び替え不要）、動画（ファイル）、リンク（公開URL・GitHub・動画URLの固定3欄＋その他のリンクを最大5件）、タグ。
- **作者を入力する欄は置かない**（§1.1 R3）。フォーム冒頭に「個人情報は載せないでください。」と明記する。
- 説明・画像・動画がすべて空のままでは送信させない（§8.2 の中身条件をクライアントでも検証する）。
- 画像は選択時にクライアントで処理（§9.2）。
- 動画ファイルは保存後に分割アップロード開始（§9.4）。フォーム本体の保存と動画は別ステップにし、本文保存 → 編集キー表示 → 動画アップロードの順。
- 投稿完了時：**作品コード**と**編集キー**を**一度だけ**大きく表示し、それぞれコピーボタンを付ける。作品コードには「アンケートでこのコードを聞きます。控えておいてください」、編集キーには「この画面を閉じると再表示できません。先生に聞けば再発行できます」と注記。同時に `localStorage.workCodes[workId]` と `localStorage.editKeys[workId]` に保存する。
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
localStorage["works.workCodes"]       // { [workId]: workCode }
localStorage["works.uploads"]         // { [workId]: { uploadId, size, ext, totalChunks } }
```

「編集キーを管理」画面（設定アイコン）から一覧・削除・手入力できる。作品コードは同じ画面に読み取り専用で並べて表示し、コピーできるようにする（アンケート記入時に見返せるように）。

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
| 作者情報の漏洩 | **そもそも保持しない**。氏名・クラス・連絡先の入力欄をどのコレクションにも置かない（§1.1 R3）。`work_secrets`（編集キー・作品コード）は全ルールロック |
| 作品コードからの逆引き | 作品コードは `work_secrets` にあり一般APIからは読めない。本人と先生しか知らない。アンケート結果は本サイトの外で保管する |
| ファイル名からの推定 | 全ファイルをサーバーで乱数名に付け替え（元名は破棄） |
| 画像メタデータ | クライアント再エンコード＋サーバー exiftool |
| 動画メタデータ | ffmpeg 再エンコードで作成日時・端末情報は落ちる。`-map_metadata -1` を追加してタイトル等も落とす |
| 投稿時刻からの推定 | 一覧の「新着順」で投稿時刻が分かる。学内用途では許容（v1）。気になる場合は表示を日付のみにする |
| Markdown XSS | DOMPurify、生HTML禁止 |
| 管理画面 `/_/` の露出 | superuser パスワードは20文字以上。Cloudflare Access で `/_/*` にメールOTP（管理者1名、無料）を掛ける（§12.4）。API の `/api/collections/_superusers/*` にはレートリミット |
| 合言葉総当たり | レートリミット（§7.4）＋ Cloudflare Rate Limiting。合言葉は8文字以上 |
| 巨大アップロードでのディスク枯渇 | `max_video_bytes`、空き容量チェック、TTL 掃除、1作品1動画 |
| 依存の脆弱性 | `npm audit` / `govulncheck` を CI で実行（任意） |
| 監査ログ | 合言葉不一致・編集キー不一致・作品削除・イベント設定変更を PocketBase の logs に `audit=true` タグ付きで記録（編集キー・作品コードは含めない） |

---

## 12. デプロイ

### 12.1 前提

- 学内サーバー：Linux（Ubuntu / Debian、x86_64 または aarch64）。systemd があること。
- メモリ 4GB 以上（swap 含む）。`/nix` 側に 20GB、データ側に 10GB 以上の空き。
- 学内サーバーからインターネットへ HTTPS（443）の外向き接続ができること。UDP/7844 が通らない場合は §12.4 の http2 フォールバックを使う。

公開ホスト名は **`ncchub.ncc-system.jp`** とする。zone `ncc-system.jp` は既に Cloudflare 管理下にあるため、ネームサーバーの移管は不要である。

zone は他用途と共有している。以下の3点に注意する。

- **既存の MX と SPF を消さない。** メールは別サーバーで受けている。
- **`*.ncc-system.jp` のワイルドカードレコードが存在する。** Tunnel の Public Hostname を設定すると `ncchub` の明示レコードが作られ、ワイルドカードより優先される。設定後に DNS タブで明示レコードになっていることを確認する。
- **zone 全体に効く設定を入れない。** 特に WAF のレートリミットは `http.host` で対象ホストを限定する（§12.4）。無料プランのレートリミットは1本だけなので、他用途と取り合いになる。

Cloudflare の無料プランでは **サブドメイン単独の zone を作れない**（subdomain setup は Enterprise 限定、partial/CNAME setup は Business 以上）。そのため zone は apex 単位で管理する前提になる。

### 12.2 インストール（`deploy/install.sh`）

学内サーバー上で次の1コマンドを実行する。Nix の導入からサービス起動までを行い、再実行しても安全（冪等）である。

```sh
curl -fsSL https://raw.githubusercontent.com/ncc-toda/ncc-hub/main/deploy/install.sh -o install.sh
sudo bash install.sh \
  --admin-email <先生のメール> \
  --generate-admin-password \
  --tunnel-token-file /path/to/token.txt \
  --event-name '文化祭くじ引きアプリ ハッカソン 2026' \
  --event-slug fes2026 \
  --event-passphrase-file /path/to/passphrase.txt
```

スクリプトが行うこと。

1. 前提検査（root、systemd、OS、アーキ、ディスク、メモリ、外向き疎通、時刻同期）
2. Nix（multi-user）の導入。既に入っていればスキップする
3. `works` システムユーザーと `/var/lib/works/pb_data` の作成
4. `/opt/works/src` へリポジトリを取得（full clone。shallow は nix の git fetcher が revision を解決できないため使わない）
5. `nix build .#default -o /opt/works/releases/<sha>` でビルドし、`/opt/works/current` を `mv -T` で原子的に切り替える
6. superuser の作成（**サービス起動前**に `runuser -u works` で実行する）
7. `deploy/works-server.service` を配置して起動し、`/api/health` で待機する
8. `--event-*` が与えられていれば `events` を1件投入する
9. `--tunnel-token*` が与えられていれば cloudflared を導入してトンネルを登録する
10. 残る手作業のチェックリストを出力する

秘密情報は `--admin-password-file` / `--tunnel-token-file` / `--event-passphrase-file` で渡すのが既定の作法とする。値を直接渡す `--admin-password` 等も受け付けるが、`ps` に露出するため警告を出す。

**リリースディレクトリ方式にする理由。** `nix build -o /opt/works/current` のように `current` を直接 out-link にすると、切り替えた瞬間に旧世代の GC ルートが失われ、ロールバック先が `nix-collect-garbage` で消える。`/opt/works/releases/<sha>` を out-link（GC ルート）にし、`current` はそれを指す素の symlink にすることで直近数世代が保護される。`releases/` 配下の symlink を手で削除してはならない。

**superuser をサービス起動前に作る理由。** SQLite を同時に開くプロセスが存在しない状態でマイグレーションと superuser 作成が完了し、かつポートが開く時点で既に superuser が存在するため、PocketBase の「最初の superuser を作成」画面が一瞬も外部へ露出しない。root で実行すると `pb_data` に root 所有のファイルが作られてサービスが書けなくなるため、`runuser -u works` は必須である。

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
TimeoutStopSec=30
LimitNOFILE=8192
# WORKS_* の既定値は SPEC §10.5 を参照。変更が必要なものだけここに書く。
Environment=WORKS_FFMPEG_THREADS=2
# ハードニング
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/works
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

サーバーが NixOS なら `deploy/nixos-module.nix` を使う。オプションは `services.works` で、`package`（必須）、`dataDir`、`listenAddr`、`environment` を取る。

### 12.3 更新（`deploy/update.sh`）

```sh
sudo /opt/works/src/deploy/update.sh
```

更新前バックアップ → fetch → ビルド → `current` の原子的切替 → restart → `/api/health` 検証、の順で進み、health が通らなければ直前のリリースへ戻して再起動する。マイグレーションは起動時に自動適用される（本番でも `migratecmd` の Automigrate は **off**、コード内 `migrations` パッケージの登録分のみ適用）。スキーマ変更はバイナリを戻しても戻らないため、更新前バックアップは既定で取る。

### 12.4 Cloudflare

トンネルは **ダッシュボード管理方式（token）** を使う。`cloudflared tunnel login` のブラウザ認証、`config.yml`、credentials ファイルがいずれも不要になり、ingress とホスト名の設定がダッシュボード側に集約される。結果として install.sh は公開ドメインに依存しない。

1. **Tunnel 作成**（Cloudflare ダッシュボード）
   Zero Trust → Networks → Tunnels → Create a tunnel → Cloudflared を選び、表示される token を控える。install.sh に `--tunnel-token-file` で渡す。
2. **Public Hostname の設定**（ダッシュボード）
   作成したトンネルの Public Hostname に公開ホスト名を設定し、Service を `http://127.0.0.1:8090` にする。
3. **ダッシュボード設定**
   - SSL/TLS：Full
   - Security → WAF → Rate limiting rules：`(http.host eq "ncchub.ncc-system.jp" and http.request.method eq "POST" and http.request.uri.path contains "/api/")` を 60 req/分 でブロック（無料枠1本）。**`http.host` の条件を必ず入れる**。zone を他用途と共有しているため、外すと同じ zone の別サイトにも掛かる
   - Zero Trust → Access → Applications：`ncchub.ncc-system.jp/_/*` と `ncchub.ncc-system.jp/api/collections/_superusers/*` に Self-hosted アプリを作り、ポリシー「メールが `<管理者のメール>` に一致 → Allow（One-time PIN）」。**それ以外のパスには Access を掛けない**（学生には合言葉のみ）
   - Caching：既定でよい。`/api/files/*` はキャッシュされても問題ない（乱数名なので更新時はURLが変わる）
4. **確認**：`curl -sI https://ncchub.ncc-system.jp/api/health` が 200。
5. **UDP が塞がれている場合**：`journalctl -u cloudflared` に `Registered tunnel connection` が出なければ、学内ファイアウォールが QUIC（UDP/7844）を遮断している。install.sh に `--tunnel-protocol http2` を付けて再実行する。

works-server は `127.0.0.1` のみで待ち受けるため、ルーターやファイアウォールで 8090 を開けてはならない。

### 12.5 バックアップ

- **日次自動**：`0 3 * * *`、保持7世代。マイグレーション（`internal/migrations`）で `settings.Backups` に設定済みで、手作業は不要。`pb_data/backups/` に zip が作られる。
- **手動**：`sudo /opt/works/src/deploy/backup.sh`。管理 API の `POST /api/backups` を叩くため、**稼働中のプロセス内で**日次バックアップと同じコードパスを通る。
  PocketBase の `CreateBackup` は zip 生成中に削除・追加された storage ファイルを同一プロセス内のフックで検出して除外する。したがって **別プロセスの CLI から実行してはならない**。アップロードや ffmpeg 変換が走っている最中に取ると、動画が途中まで書かれた状態で zip に入る。
- **コールド取得**：`deploy/backup.sh --cold` はサービスを停止して `pb_data` を tar で固める。リストア直前やマイグレーションを伴う作業の直前に使う。
- 週1回、`pb_data/backups/` を別マシン（先生のPC等）へ `rsync` する。
- **復元**：`works-server` を止め、zip を `pb_data` に展開し直す。または管理画面の Backups から復元する。

---

## 13. 運用手順（先生向け、README にも転記）

1. 管理画面 `https://ncchub.ncc-system.jp/_/` にログイン。
2. `events` に1件作成：`name`「文化祭くじ引きアプリ ハッカソン 2026」、`slug` `fes2026`、`passphrase`（8文字以上、学生に配る）、`max_video_bytes` 2147483648、`submissions_open` true。
3. 学生に `https://ncchub.ncc-system.jp/e/fes2026` と合言葉を配る。あわせて「投稿後に表示される作品コードを控えること」「投稿内容に名前を書かないこと」を伝える。
4. 誰がどれを投稿したかを知りたいとき：**本サイトには作者情報が無い**。別途アンケート（フォーム等）で氏名と作品コードを回収し、管理画面 → `work_secrets` → `work_code` で作品を特定する。アンケートの回答は本サイトと分けて保管する。
5. 編集キーを忘れた学生には `work_secrets.edit_key` を伝える。本人確認は**その学生が作品コードを言えること**を根拠にする（氏名では照合できない）。作品コードも忘れた場合は、投稿内容を本人に説明させて先生が判断する。
6. 締切後：`submissions_open` を false にする（閲覧・いいねは継続可。いいねも止めたい場合は仕様上 423 になる）。
7. 不適切な投稿：管理画面から `works` を削除（連鎖で全て消える）。
8. `events` レコードは削除しない（作品が全部消える）。

---

## 14. 実装マイルストーン

| M | 内容 | 完了条件 |
|---|---|---|
| M1 | flake.nix、justfile、`cmd/server` 骨格、マイグレーション、`just dev` で管理画面が開く | `nix develop` → `just dev` が動く。管理画面に4コレクションがある |
| M2 | 合言葉ゲート、作品一覧・詳細（テキストのみ） | 合言葉なしで 401、ありで一覧が見える |
| M3 | 作品作成・更新・削除、画像、編集キー、作品コード、EXIF 除去 | 投稿→作品コード・編集キー表示→編集→削除が通る。保存ファイル名が乱数 |
| M4 | 分割アップロード＋変換＋サムネイル＋掃除 cron | 1.5GB の .mov がスマホ回線相当（絞った回線）で完走し、リロード後に再開でき、mp4 になって再生できる |
| M5 | いいね | トグル・件数・端末ごとの状態復元 |
| M6 | デプロイ（systemd、cloudflared、Access、レートリミット、バックアップ） | 校外のスマホから投稿・閲覧できる |
| M7 | 仕上げ：モバイルUI調整、エラー文言、README | 学生2〜3名に触ってもらい詰まらない |

---

## 15. 受け入れテスト（抜粋）

- [ ] `X-Event-Key` 無し／誤りで `GET /api/collections/works/records` が 401 になる（正しい値では 200）
- [ ] `GET /api/collections/work_secrets/records` は正しい合言葉でも 403
- [ ] `works` と `work_secrets` のどちらにも氏名・クラス・連絡先のフィールドが存在しない
- [ ] `works` のレスポンスに `edit_key` / `work_code` が一切含まれない（作成時の 201 ボディを除く）
- [ ] 作品コード が `XXXX-XXXX` 形式で、`0` `1` `I` `L` `O` `U` を含まない
- [ ] 説明が空の投稿、および画像・動画・`video_url` がどれも無い投稿が 422 になる
- [ ] 動画ファイルと動画URLが両方ある作品で、詳細に両方が表示される
- [ ] 変換失敗（`video_status = failed`）でも動画URLが表示される
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

- PocketBase は v0.40.3 を使用している（`server/go.mod`）。本書の Go API 記述は当初 v0.23 系を前提に書かれていたため、実装と食い違う箇所があれば実装側を正とし、本書を更新する。
- ファイル置き換え時に PocketBase が旧ファイルを自動削除するかは M4 でテストして確定する。
- 学内サーバーの外部公開（トンネル経由）が学校の方針上問題ないかは、着手前に一度確認する。
- 単一ファイル 2GB の結合・変換で一時的に最大 ~5GB のディスクを使う。`pb_data` は十分な空きのあるパーティションに置く。
