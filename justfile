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
    shellcheck deploy/*.sh deploy/test/*.sh

test:
    cd server && go test ./...

# 管理者（superuser）作成（初回のみ、開発用）
superuser email password:
    cd server && go run ./cmd/server superuser create {{email}} {{password}} --dir=./pb_data

# 本番のバックアップは deploy/backup.sh を使う（管理 API 経由で日次 cron と同じ経路を通る）。
# 開発環境では管理画面 http://127.0.0.1:8090/_/ の Backups から取得する。
backup:
    @echo "本番: sudo /opt/works/src/deploy/backup.sh"
    @echo "開発: http://127.0.0.1:8090/_/ の Backups から取得してください"

# install.sh を systemd 入り Ubuntu コンテナで検証する（Docker Desktop が必要）
test-deploy:
    bash deploy/test/run.sh all
