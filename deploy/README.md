# deploy/

学内サーバーへ NccHub を設置・保守するための資材です。
手順の全体像は [../docs/deployment.md](../docs/deployment.md) を参照してください。

| ファイル | 用途 |
|---|---|
| `install.sh` | ワンショットインストーラ。Nix 導入からサービス起動、Tunnel 登録まで。冪等 |
| `update.sh` | 更新デプロイ。health 失敗時は直前のリリースへ自動で戻す |
| `backup.sh` | バックアップ。既定は管理 API 経由（ホット）、`--cold` で停止＋tar |
| `works-server.service` | systemd unit のテンプレート。install.sh がパスを置換して配置する |
| `nixos-module.nix` | サーバーが NixOS の場合のモジュール（`services.works`） |
| `test/` | systemd 入り Ubuntu コンテナでの検証一式 |

3 本のスクリプトは `curl | bash` で単体実行できるよう、互いを `source` しない
自己完結の構成にしています。ログ出力と health 待機のヘルパが重複するので、
変更するときは 3 本を揃えてください。

## 実行順序

```bash
sudo bash install.sh --admin-email <mail> --generate-admin-password \
                     --tunnel-token-file /root/tunnel-token.txt   # 初回
sudo ./update.sh                                                   # 更新
sudo ./backup.sh --keep 7                                          # バックアップ
```

## 検証

```bash
just test-deploy          # = bash test/run.sh all
```

Docker Desktop が必要です。初回は nix build に 15〜30 分かかります。
Cloudflare Tunnel の登録だけは本物の token が必要なため、この検証では通していません。
