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
