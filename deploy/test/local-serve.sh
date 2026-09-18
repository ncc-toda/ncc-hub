#!/usr/bin/env bash
# nix build 成果物を本番と同じフラグでこのマシンから起動する。
# Vite は使わず、--dev も付けない。SPA は pb_public から同一オリジンで出す。
#
#   bash deploy/test/local-serve.sh
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
LISTEN=${NCCHUB_PREVIEW_HTTP:-127.0.0.1:18090}
DATA_DIR=${NCCHUB_PREVIEW_DIR:-"$REPO_ROOT/server/pb_data_preview"}
ADMIN_EMAIL=preview@example.com
ADMIN_PASSWORD=ncc-hub-preview-admin-pass
EVENT_NAME='本番相当の確認用'
EVENT_SLUG=preview
EVENT_PASSPHRASE=preview-aikotoba

C_OK=$'\033[32m'; C_NG=$'\033[31m'; C_IN=$'\033[36m'; C_0=$'\033[0m'
info() { printf '%s==>%s %s\n' "$C_IN" "$C_0" "$*"; }
ok()   { printf '  %s✓%s %s\n' "$C_OK" "$C_0" "$*"; }
die()  { printf '%sFAIL%s %s\n' "$C_NG" "$C_0" "$*" >&2; exit 1; }

FAILED=0
check() {
  local desc=$1; shift
  if "$@" >/dev/null 2>&1; then
    printf '  %sPASS%s %s\n' "$C_OK" "$C_0" "$desc"
  else
    printf '  %sFAIL%s %s\n' "$C_NG" "$C_0" "$desc"; FAILED=1
  fi
}

json_get() {
  local key=$1
  python3 -c 'import json,sys; v=json.load(sys.stdin).get(sys.argv[1]); print("" if v is None else v)' "$key"
}

bin_path() { printf '%s/result/bin/works-server' "$REPO_ROOT"; }
pub_path() { printf '%s/result/share/works/pb_public' "$REPO_ROOT"; }

health_ok() {
  curl -fsS --max-time 3 "http://$LISTEN/api/health" >/dev/null 2>&1
}

ensure_build() {
  info "nix build .#default"
  (cd "$REPO_ROOT" && nix build .#default)
  [ -x "$(bin_path)" ] || die "result/bin/works-server がありません"
  [ -f "$(pub_path)/index.html" ] || die "pb_public/index.html がありません"
}

ensure_superuser() {
  mkdir -p "$DATA_DIR"
  local out
  set +e
  out=$("$(bin_path)" superuser upsert "$ADMIN_EMAIL" "$ADMIN_PASSWORD" --dir="$DATA_DIR" 2>&1)
  local rc=$?
  set -e
  [ "$rc" -eq 0 ] || die "管理者の作成に失敗しました: $out"
}

listen_port() { printf '%s' "$LISTEN" | cut -d: -f2; }

port_in_use() {
  lsof -nP -iTCP:"$(listen_port)" -sTCP:LISTEN >/dev/null 2>&1
}

start_server() {
  info "works-server を起動 ($LISTEN)"
  "$(bin_path)" serve \
    --http="$LISTEN" \
    --dir="$DATA_DIR" \
    --publicDir="$(pub_path)" &
  SERVER_PID=$!
  trap 'if [ -n "${SERVER_PID:-}" ]; then kill "$SERVER_PID" 2>/dev/null || true; fi' EXIT
}

wait_health() {
  local deadline=$(( $(date +%s) + 60 ))
  info "/api/health の応答を待機"
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if health_ok; then
      ok "health OK"
      return 0
    fi
    if [ -n "${SERVER_PID:-}" ] && ! kill -0 "$SERVER_PID" 2>/dev/null; then
      die "works-server が起動中に終了しました"
    fi
    sleep 0.3
  done
  die "/api/health が応答しません"
}

auth_token() {
  curl -fsS --max-time 20 -X POST "http://$LISTEN/api/collections/_superusers/auth-with-password" \
    -H 'Content-Type: application/json' \
    --data-binary "{\"identity\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" \
    | json_get token
}

seed_event() {
  info "イベント $EVENT_SLUG を投入"
  local token count
  token=$(auth_token) || true
  if [ -z "${token:-}" ]; then
    die "管理者として認証できませんでした"
  fi
  count=$(curl -fsS --max-time 20 -G "http://$LISTEN/api/collections/events/records" \
            --data-urlencode "filter=(slug='$EVENT_SLUG')" --data-urlencode "perPage=1" \
            -H "Authorization: $token" | json_get totalItems)
  if [ "${count:-0}" != 0 ]; then
    ok "イベント $EVENT_SLUG は既に存在します"
    return 0
  fi
  curl -fsS --max-time 20 -X POST "http://$LISTEN/api/collections/events/records" \
    -H "Authorization: $token" -H 'Content-Type: application/json' \
    --data-binary "{\"name\":\"$EVENT_NAME\",\"slug\":\"$EVENT_SLUG\",\"passphrase\":\"$EVENT_PASSPHRASE\",\"max_video_bytes\":2147483648,\"submissions_open\":true}" \
    >/dev/null
  ok "イベント $EVENT_SLUG を作成"
}

run_checks() {
  info "本番相当のアサーション"
  local pub key
  pub=$(pub_path)
  key=$EVENT_PASSPHRASE
  check "works-server が実行ファイル" test -x "$(bin_path)"
  check "ffmpeg が wrapper 経由で引ける" grep -q ffmpeg "$(bin_path)"
  check "/api/health が 200" curl -fsS --max-time 5 "http://$LISTEN/api/health"
  check "SPA が配信される" bash -lc "curl -fsS --max-time 5 'http://$LISTEN/' | grep -qiE '<div id=\"app\"|<div id=app|<script'"
  check "SPA の /e/preview が fallback する" bash -lc "curl -fsS --max-time 5 'http://$LISTEN/e/preview' | grep -qiE '<div id=\"app\"|<div id=app|<script'"
  check "pb_public に開発用バーが無い" bash -lc "! grep -Rqs '開発環境' '$pub'"
  check "合言葉なしは 401" bash -lc "[ \"\$(curl -s -o /dev/null -w %{http_code} 'http://$LISTEN/api/collections/works/records')\" = 401 ]"
  check "開発用合言葉では 401" bash -lc "[ \"\$(curl -s -o /dev/null -w %{http_code} -H 'X-Event-Key: dev-aikotoba' 'http://$LISTEN/api/collections/works/records')\" = 401 ]"
  check "合言葉ありは 200" bash -lc "[ \"\$(curl -s -o /dev/null -w %{http_code} -H 'X-Event-Key: $key' 'http://$LISTEN/api/collections/works/records')\" = 200 ]"
  check "プロセスに --dev が無い" bash -lc "! pgrep -lf 'works-server serve' | grep -F -- '--http=$LISTEN' | grep -q -- '--dev'"
  [ "$FAILED" = 0 ] || die "検証に失敗した項目があります"
  info "全項目 PASS"
}

print_urls() {
  cat <<EOF

本番相当の確認用サーバー
  URL:      http://$LISTEN/
  イベント: $EVENT_SLUG
  合言葉:   $EVENT_PASSPHRASE
  管理画面: http://$LISTEN/_/
  管理者:   $ADMIN_EMAIL / $ADMIN_PASSWORD

EOF
}

already_up=0
if health_ok; then
  already_up=1
  info "http://$LISTEN は既に応答しています"
elif port_in_use; then
  die "ポート $(listen_port) は使用中です。別プロセスを止めるか NCCHUB_PREVIEW_HTTP を変えてください"
fi

ensure_build
if [ "$already_up" = 0 ]; then
  ensure_superuser
  start_server
  wait_health
fi
seed_event
run_checks
print_urls

if [ "$already_up" = 1 ]; then
  info "既存プロセスを使いました。停止は起動元の端末で行ってください"
  trap - EXIT
  exit 0
fi

info "停止するには Ctrl-C"
wait "$SERVER_PID"
