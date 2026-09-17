#!/usr/bin/env bash
# NccHub バックアップ
#
#   sudo /opt/works/src/deploy/backup.sh              # ホット（管理 API 経由、既定）
#   sudo /opt/works/src/deploy/backup.sh --cold       # サービス停止 + tar
#
# ホットバックアップを管理 API 経由にしているのは、PocketBase の CreateBackup が
# zip 生成中に削除・追加された storage ファイルを「同一プロセス内のフック」で検出して
# 除外する設計だからである（core/backup_create.go）。別プロセスの CLI から呼ぶと
# この保護が効かず、アップロードや ffmpeg 変換の最中に取ると動画が途中まで書かれた
# 状態で zip に入る。API 経由なら日次 cron と 1 バイトも違わない経路を通る。
#
# 注意: install.sh / update.sh と共通のヘルパを持つ。変更時は 3 本を揃えること。
set -Eeuo pipefail

ENV_FILE=/etc/works/install.env
BACKUP_ENV=/etc/works/backup.env
NAME=; OUT_DIR=; KEEP=; COLD=0; URL=; TIMEOUT=1800; DRY_RUN=0; VERBOSE=0

C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_ERR=$'\033[31m'; C_IN=$'\033[36m'; C_0=$'\033[0m'
info() { printf '%s==>%s %s\n' "$C_IN" "$C_0" "$*"; }
ok()   { printf '  %s✓%s %s\n' "$C_OK" "$C_0" "$*"; }
warn() { printf '%s警告:%s %s\n' "$C_WARN" "$C_0" "$*" >&2; }
die()  { printf '%sエラー:%s %s\n' "$C_ERR" "$C_0" "$*" >&2; exit 1; }
run()  { if [ "$DRY_RUN" = 1 ]; then printf '  [dry-run] %s\n' "$*"; return 0; fi
         [ "$VERBOSE" = 1 ] && printf '    %s\n' "$*" >&2; "$@"; }

usage() {
  cat <<'USAGE'
NccHub バックアップ

  sudo backup.sh [オプション]

  --name NAME       バックアップ名。PocketBase の検証に合わせ [a-z0-9_-]+.zip のみ
  --out-dir DIR     生成物を DIR へもコピーする
  --keep N          最新 N 世代だけ残して古いものを削除する
  --cold            サービスを停止して pb_data を tar で固める
  --url URL         既定 http://<install.env の listen>
  --timeout SEC     完了待ちの上限（既定 1800）
  --dry-run         実行せず計画のみ表示
  -v, --verbose     実行コマンドを表示
  -h, --help        このヘルプ

ホット（既定）は管理 API 経由で、日次自動バックアップと同じコードパスを通る。
--cold はリストア直前やマイグレーションを伴う作業の直前に使う。
認証情報は /etc/works/backup.env（WORKS_BACKUP_EMAIL / WORKS_BACKUP_PASSWORD）から読む。
USAGE
}

while [ $# -gt 0 ]; do
  case $1 in
    --name) NAME=$2; shift 2 ;;
    --out-dir) OUT_DIR=$2; shift 2 ;;
    --keep) KEEP=$2; shift 2 ;;
    --cold) COLD=1; shift ;;
    --url) URL=$2; shift 2 ;;
    --timeout) TIMEOUT=$2; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    -v|--verbose) VERBOSE=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知のオプション: $1" ;;
  esac
done

[ "$(id -u)" = 0 ] || die "root で実行してください"
[ -r "$ENV_FILE" ] || die "$ENV_FILE がありません。先に install.sh を実行してください"
# shellcheck source=/dev/null
. "$ENV_FILE"

DATA_DIR=${WORKS_DATA_DIR:-/var/lib/works}
LISTEN=${WORKS_LISTEN:-127.0.0.1:8090}
PB_DATA=$DATA_DIR/pb_data
URL=${URL:-http://$LISTEN}

# PocketBase の検証は ^[a-z0-9_-]+\.zip$（apis/backup_create.go）。
# 大文字・コロン・余分なドットを含むと 400 になる。
NAME=${NAME:-works_$(date -u +%Y%m%d_%H%M%S).zip}
case $NAME in
  *.zip) : ;;
  *) die "--name は .zip で終わる必要があります: $NAME" ;;
esac
case ${NAME%.zip} in
  *[!a-z0-9_-]*) die "--name は英小文字・数字・アンダースコア・ハイフンのみです: $NAME" ;;
esac

# ---------------------------------------------------------------- cold
if [ "$COLD" = 1 ]; then
  OUT_DIR=${OUT_DIR:-$DATA_DIR/cold-backups}
  OUT="$OUT_DIR/${NAME%.zip}.tar.gz"
  info "コールドバックアップ: $OUT"
  run install -d -m 0700 "$OUT_DIR"
  WAS_ACTIVE=0
  systemctl is-active --quiet works-server && WAS_ACTIVE=1
  # trap から呼ぶ
  # shellcheck disable=SC2329
  restore_service() {
    if [ "$WAS_ACTIVE" = 1 ] && ! systemctl is-active --quiet works-server; then
      warn "サービスを再起動します"
      systemctl start works-server || true
    fi
  }
  trap restore_service EXIT
  [ "$WAS_ACTIVE" = 1 ] && run systemctl stop works-server
  run tar -C "$DATA_DIR" -czf "$OUT" pb_data
  run chmod 600 "$OUT"
  [ "$WAS_ACTIVE" = 1 ] && run systemctl start works-server
  trap - EXIT
  if [ "$DRY_RUN" = 0 ] && [ "$WAS_ACTIVE" = 1 ]; then
    deadline=$(( $(date +%s) + 120 ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
      curl -fsS --max-time 3 "$URL/api/health" >/dev/null 2>&1 && break
      sleep 1
    done
    if curl -fsS --max-time 3 "$URL/api/health" >/dev/null 2>&1; then
      ok "サービスは復旧しました"
    else
      warn "サービスが応答しません。journalctl を確認してください"
    fi
  fi
  ok "作成: $OUT"
  exit 0
fi

# ---------------------------------------------------------------- hot
[ -r "$BACKUP_ENV" ] || die "$BACKUP_ENV がありません。バックアップ用の認証情報が必要です（install.sh が作成します）"
# shellcheck source=/dev/null
. "$BACKUP_ENV"
: "${WORKS_BACKUP_EMAIL:?$BACKUP_ENV に WORKS_BACKUP_EMAIL がありません}"
: "${WORKS_BACKUP_PASSWORD:?$BACKUP_ENV に WORKS_BACKUP_PASSWORD がありません}"

command -v jq >/dev/null || die "jq が必要です"

if [ "$DRY_RUN" = 1 ]; then
  printf '  [dry-run] POST %s/api/backups (name=%s)\n' "$URL" "$NAME"
  [ -n "$OUT_DIR" ] && printf '  [dry-run] %s へコピー\n' "$OUT_DIR"
  [ -n "$KEEP" ]    && printf '  [dry-run] 最新 %s 世代を残して削除\n' "$KEEP"
  exit 0
fi

info "管理 API で認証"
# 秘密情報は stdin で渡す。argv に載せない。
TOKEN=$(jq -nc --arg i "$WORKS_BACKUP_EMAIL" --arg p "$WORKS_BACKUP_PASSWORD" \
          '{identity:$i,password:$p}' \
        | curl -fsS --max-time 30 -X POST "$URL/api/collections/_superusers/auth-with-password" \
            -H 'Content-Type: application/json' --data-binary @- \
        | jq -r '.token // empty')
[ -n "$TOKEN" ] || die "管理 API の認証に失敗しました"
ok "認証 OK"

info "バックアップを作成: $NAME"
# pb_data が大きいと PocketBase の WriteTimeout(5分) で接続が切れるが、
# サーバ側の CreateBackup は完走する。切断をエラー扱いせず、一覧で完了を待つ。
set +e
jq -nc --arg n "$NAME" '{name:$n}' \
  | curl -fsS --max-time "$TIMEOUT" -X POST "$URL/api/backups" \
      -H "Authorization: $TOKEN" -H 'Content-Type: application/json' --data-binary @- >/dev/null 2>&1
rc=$?
set -e
[ $rc -ne 0 ] && warn "接続が切れました（rc=$rc）。サーバ側の完了を待ちます"

deadline=$(( $(date +%s) + TIMEOUT ))
while [ "$(date +%s)" -lt "$deadline" ]; do
  if curl -fsS --max-time 30 "$URL/api/backups" -H "Authorization: $TOKEN" \
       | jq -e --arg n "$NAME" 'map(select(.key == $n)) | length > 0' >/dev/null 2>&1; then
    ok "作成完了: $NAME"
    break
  fi
  sleep 5
done
curl -fsS --max-time 30 "$URL/api/backups" -H "Authorization: $TOKEN" \
  | jq -e --arg n "$NAME" 'map(select(.key == $n)) | length > 0' >/dev/null 2>&1 \
  || die "バックアップの作成を確認できませんでした"

if [ -n "$OUT_DIR" ]; then
  info "$OUT_DIR へコピー"
  install -d -m 0700 "$OUT_DIR"
  install -m 0600 "$PB_DATA/backups/$NAME" "$OUT_DIR/$NAME"
  ok "コピー: $OUT_DIR/$NAME"
fi

if [ -n "$KEEP" ]; then
  info "最新 $KEEP 世代を残して削除"
  curl -fsS --max-time 30 "$URL/api/backups" -H "Authorization: $TOKEN" \
    | jq -r 'sort_by(.modified) | reverse | .['"$KEEP"':] | .[].key' \
    | while IFS= read -r key; do
        [ -n "$key" ] || continue
        if curl -fsS --max-time 30 -X DELETE "$URL/api/backups/$key" -H "Authorization: $TOKEN" >/dev/null; then
          ok "削除: $key"
        else
          warn "削除に失敗: $key"
        fi
      done
fi

ok "完了"
