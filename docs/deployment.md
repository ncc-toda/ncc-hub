# デプロイ・環境構築ガイド

学内 Linux サーバー上で NccHub を公開・保守する手順書です。
インストールは `deploy/install.sh` の 1 コマンドで完結します。

## 1. 構成概要

学内サーバーから Cloudflare Tunnel 経由で外向き接続します。
ルーターのポート開放や固定グローバル IP は不要です。

```
[クライアント] ──HTTPS──▶ [Cloudflare] ──Tunnel──▶ [学内サーバー]
                                                    ├─ works-server (127.0.0.1:8090)
                                                    └─ cloudflared
```

works-server は `127.0.0.1` のみで待ち受けます。
ファイアウォールで 8090 を開けてはいけません。

## 2. 前提環境

- OS: Ubuntu / Debian（x86_64 または aarch64）。systemd が動いていること
- メモリ: 4 GB 以上（swap 含む）。4 GB 未満では Go のビルドが OOM で落ちることがあります
- ディスク: `/nix` 側に 20 GB、データ側に 10 GB 以上の空き
- ネットワーク: サーバーからインターネットへの HTTPS（443）外向き接続
- Cloudflare アカウント（無料）

Nix は `install.sh` が自動で導入します。事前準備は不要です。
ffmpeg と exiftool は Nix のビルド成果物に同梱されるため、`apt install` は不要です。

学内サーバーの外部公開が学校の方針上問題ないかは、着手前に確認してください。

## 3. 公開ドメインの用意

Cloudflare の無料プランでは **サブドメイン単独の zone を作れません**。
subdomain setup は Enterprise 限定、partial（CNAME）setup は Business 以上です。
したがって現実的な経路は次の 2 つに限られます。

### 経路 A: 学校ドメイン全体を Cloudflare へ NS 委任する

学校の既存 zone をまるごと Cloudflare に移します。
MX、SPF、DKIM、既存 Web の A / CNAME をすべて移行する必要があり、
**移行ミスは学校のメールを止めます**。
情報システム管理者の承認と作業窓口が必須です。

### 経路 B: 独自ドメインを 1 つ新規取得する（推奨）

年額数百〜数千円で取得し、ネームサーバーを Cloudflare に向けます。
学校の既存 DNS には一切触りません。
校内ハッカソンという用途に対して、経路 A のリスクは釣り合いません。

### 検証用: Quick Tunnel

ドメインを用意する前に動作確認だけしたい場合は、
`--skip-tunnel` で install.sh を通してから手動でクイックトンネルを起動します。

```bash
cloudflared tunnel --url http://127.0.0.1:8090
```

`*.trycloudflare.com` のランダムな URL が発行されます。
再起動のたびに URL が変わり、**Cloudflare Access も WAF も掛けられません**。
管理画面がパスワードだけで外部公開される状態になるため、本番運用には使わないでください。

## 4. Cloudflare Tunnel の作成（ダッシュボード側）

トンネルは **ダッシュボード管理方式（token）** を使います。
`cloudflared tunnel login` のブラウザ認証、`config.yml`、credentials ファイルがいずれも不要です。

1. Cloudflare Zero Trust → Networks → Tunnels → 「Create a tunnel」
2. 「Cloudflared」を選び、トンネル名（例: `works`）を入力します
3. 表示される **token** をコピーし、サーバー上のファイルに保存します

```bash
# サーバー上で
umask 077
cat > /root/tunnel-token.txt   # token を貼り付けて Ctrl-D
```

Public Hostname の設定はインストール後に行います（§6）。

## 5. インストール

サーバー上で次を実行します。

```bash
curl -fsSL https://raw.githubusercontent.com/ncc-toda/ncc-hub/main/deploy/install.sh -o install.sh
sudo bash install.sh \
  --admin-email <先生のメールアドレス> \
  --generate-admin-password \
  --tunnel-token-file /root/tunnel-token.txt \
  --event-name '文化祭くじ引きアプリ ハッカソン 2026' \
  --event-slug fes2026 \
  --event-passphrase-file /root/passphrase.txt
```

初回は Nix の導入とビルドで 15〜30 分かかります。
再実行しても安全です（冪等）。変更が無ければサービスの再起動も行いません。

### 主なオプション

| オプション | 説明 |
|---|---|
| `--admin-email EMAIL` | 管理者（superuser）のメール |
| `--admin-password-file PATH` | 管理者パスワードをファイルから読む（推奨） |
| `--generate-admin-password` | 32 文字のパスワードを自動生成する |
| `--force-admin-password` | 既存管理者のパスワードを上書きする |
| `--tunnel-token-file PATH` | Tunnel token をファイルから読む（推奨） |
| `--tunnel-protocol http2` | UDP/7844 が塞がれている環境で使う |
| `--skip-tunnel` | トンネル設定を行わない |
| `--event-slug SLUG` | 初期イベントを投入する（合言葉の指定も必要） |
| `--dry-run` | 実行せず計画のみ表示する |
| `-y` | 対話プロンプトを出さない |

パスワードと token は `--*-file` で渡すのが既定の作法です。
値を直接渡すオプションもありますが、`ps` に露出するため警告が出ます。

管理者パスワードは 20 文字以上、イベントの合言葉は 8 文字以上が必要です。

### install.sh が行うこと

1. 前提検査（root、systemd、OS、アーキ、ディスク、メモリ、外向き疎通、時刻同期）
2. Nix（multi-user）の導入。導入済みならスキップ
3. `works` システムユーザーと `/var/lib/works/pb_data` の作成
4. `/opt/works/src` へリポジトリを取得
5. `nix build` でビルドし `/opt/works/current` を原子的に切り替え
6. 管理者（superuser）の作成。**サービス起動前**に `runuser -u works` で実行
7. バックアップ専用 superuser の作成と `/etc/works/backup.env` の書き出し
8. systemd unit の配置と起動、`/api/health` での待機
9. `--event-*` があれば `events` を 1 件投入
10. cloudflared の導入とトンネル登録
11. 残る手作業のチェックリストを出力

### 生成されるファイル

| パス | 内容 |
|---|---|
| `/opt/works/src` | リポジトリのクローン |
| `/opt/works/releases/<sha>` | 各リリース（Nix の GC ルート。**手で消さないこと**） |
| `/opt/works/current` | 現行リリースへの symlink |
| `/var/lib/works/pb_data` | PocketBase のデータ |
| `/etc/works/install.env` | 設置時の設定（秘密情報は含まない） |
| `/etc/works/backup.env` | バックアップ用の認証情報（0600） |
| `/etc/works/state/` | インストーラの状態マーカー |
| `/root/ncc-hub-install-secrets.txt` | 自動生成した管理者パスワード（控えたら `shred -u`） |

`/opt/works/releases/<sha>` を Nix の GC ルートにしているのは、
`current` を直接 `nix build -o` の出力先にすると、
切り替えた瞬間に旧世代の GC ルートが失われ、
ロールバック先が `nix-collect-garbage` で消えてしまうためです。

## 6. インストール後の手作業

install.sh が最後にチェックリストを出力します。以下を Cloudflare 側で設定します。

### 6.1 Public Hostname

Zero Trust → Networks → Tunnels → 該当トンネル → Public Hostname。
公開ホスト名を設定し、Service を `http://127.0.0.1:8090` にします。

### 6.2 SSL/TLS

ダッシュボード → SSL/TLS → 暗号化モードを **Full** にします。

### 6.3 WAF レート制限ルール

書き込み API への乱打を防ぎます。Security → WAF → Rate limiting rules。

- 条件: `(http.request.method eq "POST" and http.request.uri.path contains "/api/")`
- 制限: 60 リクエスト / 1 分
- 動作: Block

### 6.4 Cloudflare Access（管理画面の保護）

Zero Trust → Access → Applications → Add an application → Self-hosted。
以下の 2 パスをそれぞれ保護対象にします。

- `<公開ホスト名>/_/*`
- `<公開ホスト名>/api/collections/_superusers/*`

ポリシーは「Include: Emails に教員のメールアドレス」「認証方式は One-time PIN」。

**他の公開パスには絶対に Access を掛けないでください。**
学生向けの公開パスはアプリ層の合言葉だけで守ります。

### 6.5 疎通確認

```bash
curl -sI https://<公開ホスト名>/api/health
```

200 が返れば公開完了です。

### 6.6 イベントの作成

`--event-slug` を指定しなかった場合は、管理画面から作成します。
手順は [admin-guide.md](./admin-guide.md) を参照してください。

## 7. 更新

```bash
sudo /opt/works/src/deploy/update.sh
```

更新前バックアップ → fetch → ビルド → `current` の切替 → restart → `/api/health` 検証、の順です。
health が通らなければ **直前のリリースへ自動で戻して再起動します**。

マイグレーションは起動時に自動適用されます。
スキーマ変更はバイナリを戻しても戻らないため、更新前バックアップは既定で取得します
（`--skip-backup` で抑止できますが推奨しません）。

| オプション | 説明 |
|---|---|
| `--ref REF` | 取得する ref（既定は install 時の値） |
| `--force` | 変更が無くても再ビルドして切り替える |
| `--skip-backup` | 更新前バックアップを取らない |
| `--no-rollback` | health 失敗時に自動で戻さない |
| `--dry-run` | 実行せず計画のみ表示 |

## 8. バックアップとリストア

### 8.1 日次自動バックアップ

毎日午前 3:00 に実行され、最新 7 世代が `pb_data/backups/` に保存されます。
マイグレーションで設定済みのため、手作業は不要です。

### 8.2 手動バックアップ

```bash
sudo /opt/works/src/deploy/backup.sh --keep 7
```

管理 API の `POST /api/backups` を叩くため、**稼働中のプロセス内で**
日次バックアップとまったく同じコードパスを通ります。

PocketBase の `CreateBackup` は、zip の生成中に削除・追加された storage ファイルを
同一プロセス内のフックで検出して除外する設計です。
別プロセスの CLI から呼ぶとこの保護が効かず、
アップロードや ffmpeg 変換の最中に取ると動画が途中まで書かれた状態で zip に入ります。
そのため CLI からの直接実行は用意していません。

pb_data が大きいと PocketBase の WriteTimeout（5 分）で接続が切れますが、
サーバー側の処理は完走します。backup.sh はこれをエラー扱いせず、一覧で完了を待ちます。

| オプション | 説明 |
|---|---|
| `--name NAME` | バックアップ名。`[a-z0-9_-]+.zip` のみ（PocketBase の検証に合わせる） |
| `--out-dir DIR` | 生成物を DIR へもコピーする |
| `--keep N` | 最新 N 世代だけ残して古いものを削除する |
| `--cold` | サービスを停止して pb_data を tar で固める |

### 8.3 コールドバックアップ

```bash
sudo /opt/works/src/deploy/backup.sh --cold --out-dir /root/backups
```

サービスを停止して `pb_data` をバイト単位で保全します。
リストア直前や、マイグレーションを伴う危険な作業の直前に使います。
停止中はサイトにアクセスできません。

### 8.4 別マシンへの退避

週 1 回、別マシン（先生の PC 等）へ退避します。

```bash
rsync -av <サーバー>:/var/lib/works/pb_data/backups/ ~/works-backups/
```

### 8.5 リストア

管理画面の Backups から復元するのが確実です。
手動で行う場合はサービスを停止してから展開します。

```bash
sudo systemctl stop works-server
cd /var/lib/works/pb_data
sudo unzip -o backups/<backup-file>.zip
sudo chown -R works:works /var/lib/works
sudo systemctl start works-server
```

## 9. 運用とトラブルシュート

```bash
systemctl status works-server        # 状態確認
journalctl -u works-server -f        # ログ追跡
systemctl status cloudflared
journalctl -u cloudflared -n 50
```

### トンネルが張れない

`journalctl -u cloudflared` に `Registered tunnel connection` が出ない場合、
学内ファイアウォールが QUIC（UDP/7844）を遮断している可能性があります。

```bash
sudo bash install.sh --tunnel-protocol http2 --tunnel-token-file /root/tunnel-token.txt ...
```

### ビルドが OOM で落ちる

`modernc.org/libc` のコンパイルはメモリを多く使います。
メモリ 4 GB 未満の環境では swap を 4 GB 追加してください。

```bash
sudo fallocate -l 4G /swapfile && sudo chmod 600 /swapfile
sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

### `/tmp` が noexec

Nix のインストーラは tarball を展開して実行します。
`/tmp` が noexec でマウントされていても、install.sh は `/var/tmp` へ自動で切り替えます。
両方とも noexec の場合は失敗するので、どちらかを exec 可能にしてください。

### 学生が 401 になる

`events` レコードが 1 件も無いと、学生のすべてのリクエストが 401 になります。
管理画面で events を作成してください。

### ディスクが逼迫している

2 GB の動画 1 本の変換で一時的に約 5 GB を使います。
`pb_data` は十分な空きのあるパーティションに置いてください。
古いリリースは `--keep-releases`（既定 3）を超えた分が自動削除されます。

## 10. 付録 A: NixOS の場合

NixOS ではリポジトリの Flake module を利用できます。
オプション名は `services.works` で、`package` は必須です。

```nix
{
  inputs.ncc-hub.url = "github:ncc-toda/ncc-hub";

  # configuration.nix 側
  imports = [ inputs.ncc-hub.nixosModules.default ];
  services.works = {
    enable = true;
    package = inputs.ncc-hub.packages.${pkgs.system}.default;
    # dataDir = "/var/lib/works";       # 既定値
    # listenAddr = "127.0.0.1:8090";    # 既定値
    # environment = { WORKS_FFMPEG_THREADS = "2"; };
  };
}
```

superuser の作成と Cloudflare Tunnel の設定は §5〜§6 と同じ手順を手動で行います。

## 11. 付録 B: install.sh を使わない手動インストール

```bash
sudo useradd --system --shell /usr/sbin/nologin --home-dir /var/lib/works works
sudo install -d -o works -g works -m 0750 /var/lib/works /var/lib/works/pb_data
sudo git clone https://github.com/ncc-toda/ncc-hub.git /opt/works/src
cd /opt/works/src
sudo nix build .#default -o /opt/works/releases/manual
sudo ln -sfn /opt/works/releases/manual /opt/works/current
sudo runuser -u works -- /opt/works/current/bin/works-server \
  superuser create <email> <20文字以上のパスワード> --dir=/var/lib/works/pb_data
sudo cp deploy/works-server.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now works-server
```

superuser の作成を `runuser -u works` で行うのは必須です。
root で実行すると `pb_data` に root 所有のファイルができ、以後サービスが書けなくなります。

## 12. 付録 C: locally-managed トンネル（token を使わない場合）

ダッシュボードを使わず、サーバー上で完結させる方式です。
ブラウザ認証が必要なため自動化できません。

```bash
cloudflared tunnel login
cloudflared tunnel create works
cloudflared tunnel route dns works works.example.jp
```

`cloudflared tunnel create` は credentials を `~/.cloudflared/<TUNNEL_ID>.json` に書きます。
これを `/etc/cloudflared/` へコピーして権限を絞ります。

```bash
sudo install -d -m 0700 /etc/cloudflared
sudo install -m 0600 ~/.cloudflared/<TUNNEL_ID>.json /etc/cloudflared/
```

`/etc/cloudflared/config.yml` を作成します。

```yaml
tunnel: <TUNNEL_ID>
credentials-file: /etc/cloudflared/<TUNNEL_ID>.json
ingress:
  - hostname: works.example.jp
    service: http://127.0.0.1:8090
  - service: http_status:404
```

```bash
sudo cloudflared service install
sudo systemctl enable --now cloudflared
```

## 13. 付録 D: アンインストール

```bash
sudo systemctl disable --now works-server cloudflared
sudo rm -f /etc/systemd/system/works-server.service
sudo cloudflared service uninstall
sudo systemctl daemon-reload
sudo rm -rf /opt/works /etc/works
# データを消してよい場合のみ
sudo rm -rf /var/lib/works
sudo userdel works
```

Nix 本体を消す場合は Nix のドキュメントに従ってください。
