# デプロイ・環境構築ガイド

学内 Linux サーバー上で NccHub を公開・保守する手順書です。

## 1. 構成概要

学内サーバーから Cloudflare Tunnel 経由で外向き接続します。
ルーターのポート開放や固定グローバル IP は不要です。

```
[クライアント] ──HTTPS──▶ [Cloudflare] ──Tunnel──▶ [学内サーバー]
                                                    ├─ works-server (:8090)
                                                    └─ cloudflared
```

## 2. 前提環境

以下の環境を前提とします。

- OS: Linux（Ubuntu / Debian または NixOS）
- ツール: Nix（Flakes 有効）
- Nix を使わない場合: Go、ffmpeg、libimage-exiftool-perl

## 3. systemd によるデプロイ手順

### 3.1 ディレクトリと専用ユーザーの作成

```bash
sudo useradd -r -s /usr/sbin/nologin works
sudo mkdir -p /var/lib/works/pb_data
sudo chown -R works:works /var/lib/works
```

### 3.2 アプリケーションの取得とビルド

```bash
sudo git clone <リポジトリURL> /opt/works/src
cd /opt/works/src
sudo nix build .#default -o /opt/works/current
```

### 3.3 サービスの登録と起動

付属の systemd unit ファイルを配置します。

```bash
sudo cp deploy/works-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now works-server
```

### 3.4 管理者アカウントの初期作成

初回のみスーパーユーザーを作成します。

```bash
/opt/works/current/bin/works-server superuser create <email> <password> --dir=/var/lib/works/pb_data
```

パスワードは 20 文字以上を指定してください。

## 4. NixOS によるデプロイ

NixOS ではリポジトリの Flake module を利用できます。

```nix
{
  imports = [ ncc-hub.nixosModules.default ];
  services.works-server = {
    enable = true;
    domain = "works.example.jp";
    port = 8090;
  };
}
```

## 5. Cloudflare Tunnel の設定

学内サーバーから安全に外部公開するためのトンネルを構成します。

### 5.1 トンネルの作成

```bash
cloudflared tunnel login
cloudflared tunnel create works
cloudflared tunnel route dns works works.example.jp
```

### 5.2 設定ファイルの配置

設定例を `/etc/cloudflared/config.yml` に配置します。
元ファイルは `deploy/cloudflared.config.yml.example` です。
トンネル ID と認証ファイルのパスを書き換えます。

```bash
sudo cloudflared service install
sudo systemctl start cloudflared
```

## 6. Cloudflare ダッシュボードの設定

### 6.1 WAF レート制限ルール

書き込み API への乱打を防ぐため、WAF で制限します。
「Security」→「WAF」→「Rate limiting rules」を開きます。

- 条件: `(http.request.method eq "POST" and http.request.uri.path contains "/api/")`
- 制限: 60 リクエスト / 1 分
- 動作: Block

### 6.2 Cloudflare Access（管理画面の保護）

一般の学生が管理画面へ侵入できないよう保護します。
Cloudflare One ダッシュボードを開きます。

1. Access から「Add an application」を選択します。
2. 「Self-hosted」を選択します。
3. 以下のパスを保護対象にします。
   - `works.example.jp/_/*`
   - `works.example.jp/api/collections/_superusers/*`
4. 教員のメールアドレス（Include: Emails）を許可します。
5. 認証方式にメールワンタイム PIN を設定します。

他の公開パスには絶対に Access を掛けないでください。

## 7. バックアップとリストア

### 7.1 日次自動バックアップ

毎日午前 3:00 に自動バックアップが実行されます。
最新 7 世代が `pb_data/backups/` に保存されます。

### 7.2 手動バックアップ

作業前などに手動でバックアップを取得できます。

```bash
cd /opt/works/src
nix develop -c just backup
```

### 7.3 バックアップの別マシン退避

定期的にバックアップファイルを学外または別端末へ退避します。

```bash
rsync -av works-server:/var/lib/works/pb_data/backups/ ~/works-backups/
```

### 7.4 データのリストア

復元を行う場合はサービスを停止してから展開します。

```bash
sudo systemctl stop works-server
cd /var/lib/works/pb_data
sudo unzip -o backups/<backup-file>.zip
sudo systemctl start works-server
```

## 8. アプリケーションの更新

リポジトリ更新時のデプロイ手順です。

```bash
cd /opt/works/src
sudo git pull
sudo nix build .#default -o /opt/works/current
sudo systemctl restart works-server
```

マイグレーションはサーバー起動時に自動適用されます。
