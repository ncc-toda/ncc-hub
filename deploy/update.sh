#!/usr/bin/env bash
# NccHub 更新デプロイ
#
#   sudo /opt/works/src/deploy/update.sh
#
# fetch → ビルド → current の原子的切替 → restart → health 検証。
# health が通らなければ直前のリリースへ戻して再起動する。
# 注意: install.sh / backup.sh と共通のヘルパを持つ。変更時は 3 本を揃えること。
set -Eeuo pipefail

ENV_FILE=/etc/works/install.env
REF=; FORCE=0; SKIP_BACKUP=0; NO_ROLLBACK=0; DRY_RUN=0; VERBOSE=0

C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_ERR=$'\033[31m'; C_IN=$'\033[36m'; C_0=$'\033[0m'
info() { printf '%s==>%s %s\n' "$C_IN" "$C_0" "$*"; }
ok()   { printf '  %s✓%s %s\n' "$C_OK" "$C_0" "$*"; }
warn() { printf '%s警告:%s %s\n' "$C_WARN" "$C_0" "$*" >&2; }
die()  { printf '%sエラー:%s %s\n' "$C_ERR" "$C_0" "$*" >&2; exit 1; }
run()  { if [ "$DRY_RUN" = 1 ]; then printf '  [dry-run] %s\n' "$*"; return 0; fi
         [ "$VERBOSE" = 1 ] && printf '    %s\n' "$*" >&2; "$@"; }

usage() {
  cat <<'USAGE'
NccHub 更新デプロイ

  sudo update.sh [オプション]

  --ref REF        取得する ref（既定は /etc/works/install.env の値）
  --force          変更が無くても再ビルドして切り替える
  --skip-backup    更新前バックアップを取らない（非推奨）
  --no-rollback    health 失敗時に自動で戻さない
  --dry-run        実行せず計画のみ表示
  -v, --verbose    実行コマンドを表示
  -h, --help       このヘルプ

マイグレーションは起動時に自動適用される。スキーマ変更はバイナリを戻しても
戻らないため、更新前バックアップは既定で取得する。
USAGE
}

while [ $# -gt 0 ]; do
  case $1 in
    --ref) REF=$2; shift 2 ;;
    --force) FORCE=1; shift ;;
    --skip-backup) SKIP_BACKUP=1; shift ;;
    --no-rollback) NO_ROLLBACK=1; shift ;;
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

PREFIX=${WORKS_PREFIX:-/opt/works}
LISTEN=${WORKS_LISTEN:-127.0.0.1:8090}
KEEP_RELEASES=${WORKS_KEEP_RELEASES:-3}
REF=${REF:-${WORKS_REF:-main}}
SRC_DIR=$PREFIX/src

source_nix_profile() {
  local p=/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
  # shellcheck source=/dev/null
  if [ -r "$p" ]; then set +u; . "$p"; set -u; fi
}
source_nix_profile
NIX_BIN=$(command -v nix 2>/dev/null || echo /nix/var/nix/profiles/default/bin/nix)
[ -x "$NIX_BIN" ] || die "nix が見つかりません: $NIX_BIN"
nix_run() { run "$NIX_BIN" --extra-experimental-features "nix-command flakes" "$@"; }

dump_diagnostics() {
  printf '\n%s--- journalctl -u works-server ---%s\n' "$C_ERR" "$C_0" >&2
  journalctl -u works-server -n 120 --no-pager 2>&1 | tail -60 >&2 || true
}

wait_health() {
  [ "$DRY_RUN" = 1 ] && { printf '  [dry-run] /api/health を待機\n'; return 0; }
  local deadline=$(( $(date +%s) + ${1:-120} ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    curl -fsS --max-time 3 "http://$LISTEN/api/health" >/dev/null 2>&1 && { ok "health OK"; return 0; }
    systemctl is-active --quiet works-server || { warn "works-server が停止しました"; break; }
    sleep 1
  done
  return 1
}

switch_to() { # switch_to <releases 配下のパス>
  ln -sfn "$1" "$PREFIX/.current.tmp"
  mv -Tf "$PREFIX/.current.tmp" "$PREFIX/current"
}

prune_releases() {
  [ "$DRY_RUN" = 1 ] && return 0
  local cur n=0 r
  cur=$(readlink "$PREFIX/current" 2>/dev/null || true)
  while IFS= read -r r; do
    [ -n "$r" ] || continue
    [ "$r" = "$cur" ] && continue
    n=$((n + 1)); [ "$n" -lt "$KEEP_RELEASES" ] && continue
    rm -f "$r"
  done < <(find "$PREFIX/releases" -maxdepth 1 -mindepth 1 -type l -printf '%T@ %p\n' 2>/dev/null \
            | sort -rn | cut -d' ' -f2-)
}

PREV_TARGET=$(readlink "$PREFIX/current" 2>/dev/null || true)
CUR_SHA=$(basename "${PREV_TARGET:-}" 2>/dev/null || true)

info "更新前の確認"
ok "現在のリリース: ${CUR_SHA:-なし}"

if [ "$SKIP_BACKUP" = 0 ]; then
  info "更新前バックアップ"
  if [ -x "$SRC_DIR/deploy/backup.sh" ]; then
    run "$SRC_DIR/deploy/backup.sh" --name "pre_update_$(date -u +%Y%m%d_%H%M%S).zip" \
      || warn "バックアップに失敗しました。--skip-backup で強行できますが推奨しません"
  else
    warn "$SRC_DIR/deploy/backup.sh がありません。バックアップをスキップします"
  fi
fi

info "ソースを更新: $REF"
run git -C "$SRC_DIR" fetch --prune origin "$REF"
run git -C "$SRC_DIR" reset --hard FETCH_HEAD

if [ "$DRY_RUN" = 1 ]; then
  printf '  [dry-run] ビルドと切替を実行\n'; exit 0
fi

NEW_SHA=$(git -C "$SRC_DIR" rev-parse --short=12 HEAD)
if [ "$NEW_SHA" = "$CUR_SHA" ] && [ "$FORCE" = 0 ]; then
  ok "すでに最新です（$NEW_SHA）。--force で再ビルドできます"
  exit 0
fi

info "ビルド: $NEW_SHA"
nix_run build "$SRC_DIR#default" -o "$PREFIX/releases/$NEW_SHA" --print-build-logs

info "切り替えて再起動"
switch_to "$PREFIX/releases/$NEW_SHA"
systemctl restart works-server

if wait_health 120; then
  prune_releases
  ok "更新完了: ${CUR_SHA:-なし} -> $NEW_SHA"
  exit 0
fi

dump_diagnostics
if [ "$NO_ROLLBACK" = 1 ] || [ -z "$PREV_TARGET" ]; then
  die "health が通りませんでした。手動で復旧してください（前リリース: ${PREV_TARGET:-不明}）"
fi

warn "health が通らないため直前のリリースへ戻します: $CUR_SHA"
switch_to "$PREV_TARGET"
systemctl restart works-server
if wait_health 120; then
  die "更新に失敗したため $CUR_SHA へ戻しました。サービスは復旧しています"
fi
dump_diagnostics
die "ロールバック後も health が通りません。手動での対応が必要です"
