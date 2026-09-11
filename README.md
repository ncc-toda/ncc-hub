# 作品ひろば — 学内作品共有サイト（学内版ProtoPedia）

校内ハッカソンの制作物（画像・動画・アピール文）を投稿・閲覧・「いいね」できるサイト。
学生同士は匿名、先生（管理者）だけが作者を確認できます。仕様の詳細は [SPEC.md](./SPEC.md) を参照。

- バックエンド: PocketBase を組み込んだ Go 単一バイナリ（`server/`）
- フロントエンド: Vite + Svelte 5 + TypeScript（`web/`）
- 公開: 学内サーバー + Cloudflare Tunnel
- 開発環境: Nix flake（`nix develop`）

## 開発

```sh
# 必要なもの: Nix（flakes 有効）+ direnv（任意）
nix develop          # go / node / ffmpeg / exiftool / just などが入る
just dev             # server(127.0.0.1:8090) と web(127.0.0.1:5173) を同時起動
just superuser admin@example.com <20文字以上のパスワード>   # 初回のみ
```

- 開発中は http://127.0.0.1:5173 を開く（`/api` と `/_/` は 8090 へプロキシ）。
- 管理画面は http://127.0.0.1:8090/_/
- `just lint` / `just test` / `just build`（`nix build .#default`）

## デプロイ（学内サーバー、SPEC §12 の要約）

### 1. ビルドと systemd 登録

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

Nix が無いサーバーでは開発機で `nix build` した `result/bin/works-server`（静的バイナリ）と
`result/share/works/pb_public` を scp し、`apt install ffmpeg libimage-exiftool-perl` を入れる。

更新: `cd /opt/works/src && git pull && nix build .#default -o /opt/works/current && sudo systemctl restart works-server`
（マイグレーションは起動時に自動適用）

サーバーが NixOS の場合は `deploy/nixos-module.nix`（flake の `nixosModules.default`）が使える。

### 2. Cloudflare Tunnel

```sh
cloudflared tunnel login
cloudflared tunnel create works
cloudflared tunnel route dns works works.example.jp
```

`deploy/cloudflared.config.yml.example` を `/etc/cloudflared/config.yml` に置いて
`<TUNNEL_ID>` を書き換え、`sudo cloudflared service install`（または `deploy/cloudflared.service`）。

### 3. Cloudflare ダッシュボード

- **Rate limiting**（Security → WAF）: `(http.request.method eq "POST" and http.request.uri.path contains "/api/")` を 60 req/分 でブロック（無料枠1本）
- **Access**（Zero Trust）: `works.example.jp/_/*` と `works.example.jp/api/collections/_superusers/*` に
  Self-hosted アプリを作成し、管理者メール一致 → Allow（One-time PIN）。**他のパスには掛けない**
- 確認: `curl -sI https://works.example.jp/api/health` が 200

### 4. バックアップ

- 毎日 3:00 に自動バックアップ（7世代保持、マイグレーションで設定済み）→ `pb_data/backups/`
- 週1回、`pb_data/backups/` を別マシンへコピーする（例）:
  `rsync -av works-server:/var/lib/works/pb_data/backups/ ~/works-backups/`
- 復元: サービスを止め、zip を `pb_data` に展開して再起動

## 運用手順（先生向け、SPEC §13）

1. 管理画面 `https://works.example.jp/_/` にログイン。
2. `events` に1件作成: `name`「文化祭くじ引きアプリ ハッカソン 2026」、`slug` `fes2026`、
   `passphrase`（8文字以上、学生に配る）、`max_video_bytes` 2147483648、`submissions_open` true。
3. 学生に `https://works.example.jp/e/fes2026` と合言葉を配る。
4. 作者を確認したいとき: 管理画面 → `work_secrets` → `work` で絞り込む。
5. 編集キーを忘れた学生には `work_secrets.edit_key` を伝える（本人確認は先生の判断）。
6. 締切後: `events.submissions_open` を false にする（閲覧・いいねは継続可…いいねも仕様上 423 で止まる）。
7. 不適切な投稿: 管理画面から `works` を削除（作者情報・いいね・ファイルも連鎖削除）。
8. **`events` レコードは削除しない**（作品が全部消える）。

## ディレクトリ

```
server/   Go サーバー（PocketBase 組み込み、/api/x/ カスタムAPI、ffmpeg 変換 cron）
web/      Svelte 5 フロントエンド（ビルド成果物を pb_public として配信）
deploy/   systemd unit / cloudflared 設定例 / NixOS モジュール
```
