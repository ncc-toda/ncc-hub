# NccHub

校内ハッカソンの制作物を投稿・閲覧・「いいね」できる学内共有サイトです。
学生同士は完全匿名です。サイトは作者を特定する情報を保持せず、
投稿ごとに発行する「作品コード」と別途実施するアンケートで、教員が作者を突き合わせます。

## 特徴

- **作者情報を持たない**: 氏名・クラス・連絡先の入力欄がありません。作者の特定は作品コードとアンケートで行います。
- **大容量動画**: 分割アップロードにより最大 2 GB の動画を投稿できます。
- **自動変換**: iPhone の画面収録等の動画を再生互換性の高い mp4 へ自動変換します。
- **合言葉アクセス**: イベント単位の合言葉で学外からの不正利用を防止します。
- **軽量運用**: SQLite 内蔵の Go 単一バイナリで動作し、低コストで運用できます。

## 技術スタック

- バックエンド: PocketBase 組み込み Go 単一バイナリ（`server/`）
- フロントエンド: Vite + Svelte 5 + TypeScript（`web/`）
- 公開インフラ: 学内 Linux サーバー + Cloudflare Tunnel
- 開発環境: Nix flake（`nix develop`）

## デプロイ

学内 Linux サーバーへの設置は 1 コマンドで完結します。

```bash
curl -fsSL https://raw.githubusercontent.com/ncc-toda/ncc-hub/main/deploy/install.sh -o install.sh
sudo bash install.sh --admin-email <先生のメール> --generate-admin-password \
                     --hostname ncchub.ncc-system.jp \
                     --tunnel-token-file /root/tunnel-token.txt
```

Nix の導入、ビルド、systemd 登録、管理者作成、Cloudflare Tunnel の登録までを行い、
最後に Cloudflare ダッシュボード側で必要な手作業をチェックリストとして出力します。
再実行しても安全です。手順の詳細は [docs/deployment.md](./docs/deployment.md) を参照してください。

## 開発クイックスタート

開発環境のツールチェーンは Nix flake で固定されています。

```bash
# 開発環境に入る（go / node / ffmpeg / exiftool / just が揃います）
nix develop

# server (127.0.0.1:8090) と web (127.0.0.1:5173) を同時起動
just dev
```

`just dev` は開発用イベント `dev`（合言葉 `dev-aikotoba`）とサンプル作品を投入します。
ブラウザで http://127.0.0.1:5173 を開くと `/e/dev` に入ります。

- フロントエンド: http://127.0.0.1:5173（`/api` と `/_/` は 8090 へプロキシ）
- PocketBase 管理画面: http://127.0.0.1:8090/_/
- 開発用管理者: `dev@example.com` / `ncc-hub-dev-admin-pass`（`pb_data` が空のときだけ作成）

本番と同じ成果物で確認するときは `just preview` を使います。
`nix build` のバイナリが `pb_public` から SPA を配信します。
`--dev` は付きません。待ち受けは http://127.0.0.1:18090 です。

- イベント: `preview` / 合言葉 `preview-aikotoba`
- 管理者: `preview@example.com` / `ncc-hub-preview-admin-pass`
- データ: `server/pb_data_preview/`（`just dev` の `pb_data` とは別）

Linux 上の systemd と `install.sh` の検証は `just test-deploy` です。Docker Desktop が必要です。

別の管理者を足すときは `just superuser <email> <20文字以上のパスワード>` を使います。

## コマンド一覧

すべてのコマンドは `justfile` を起点に実行します。

| コマンド | 説明 |
|---|---|
| `just dev` | server と web の開発サーバーを同時起動します。 |
| `just lint` | `golangci-lint` と `svelte-check` を実行します。 |
| `just test` | Go のユニットテストを実行します。 |
| `just build` | `nix build .#default` で成果物を生成します。 |
| `just preview` | 本番成果物をこのマシンで起動します（Vite なし、`--dev` なし）。 |
| `just backup` | バックアップの取得方法を案内します（本番は `deploy/backup.sh`）。 |
| `just superuser <email> <password>` | 管理者アカウントを追加します。空の `pb_data` では `just dev` が `dev@example.com` を作ります。 |
| `just test-deploy` | `deploy/install.sh` を Ubuntu コンテナで検証します。 |

## ドキュメント

用途や対象読者ごとにドキュメントを整備しています。

| ドキュメント | 対象読者 | 内容 |
|---|---|---|
| [SPEC.md](./SPEC.md) | 開発者・AI | システム仕様の正本（データモデル、API 仕様等） |
| [docs/admin-guide.md](./docs/admin-guide.md) | 教員・イベント運営 | イベント作成、作品コードでの作者照会、締切、不適切投稿の対応 |
| [docs/deployment.md](./docs/deployment.md) | インフラ担当 | 学内サーバー構築、Cloudflare 設定、バックアップ |

## ディレクトリ構成

```
.
├── server/     # Go バックエンド（PocketBase 組み込み、ffmpeg 変換、cron）
├── web/        # Svelte 5 フロントエンド（SPA）
├── deploy/     # install.sh / update.sh / backup.sh、systemd unit、NixOS モジュール
├── docs/       # 先生向け運用マニュアル、デプロイ手順書
├── flake.nix   # 開発環境およびパッケージ定義
├── justfile    # 開発用コマンド定義
└── SPEC.md     # システム仕様書（正本）
```
