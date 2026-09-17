#!/usr/bin/env bash
# NccHub の deploy/*.sh を systemd 入り Ubuntu コンテナで検証するドライバ。
# macOS ホスト側で実行する。Docker Desktop が起動している必要がある。
#
#   bash deploy/test/run.sh all        # 一気通貫（初回は nix build に 15-30 分）
#   bash deploy/test/run.sh sync install verify   # 反復
#
# dexb に渡す文字列はコンテナ内で評価するため、単一引用符のままにする。
# shellcheck disable=SC2016
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
IMAGE=ncchub-test:base
SNAPSHOT=ncchub-test:installed
NAME=${NCCHUB_TEST_CONTAINER:-ncchub-test}
PLATFORM=${NCCHUB_TEST_PLATFORM:-linux/arm64}
ADMIN_EMAIL=admin@example.com
EVENT_SLUG=test2026
EVENT_PASSPHRASE=test-passphrase

C_OK=$'\033[32m'; C_NG=$'\033[31m'; C_IN=$'\033[36m'; C_0=$'\033[0m'
info() { printf '%s==>%s %s\n' "$C_IN" "$C_0" "$*"; }
ok()   { printf '  %s✓%s %s\n' "$C_OK" "$C_0" "$*"; }
die()  { printf '%sFAIL%s %s\n' "$C_NG" "$C_0" "$*" >&2; exit 1; }

FAILED=0
check() { # check <説明> <コマンド...>
  local desc=$1; shift
  if "$@" >/dev/null 2>&1; then
    printf '  %sPASS%s %s\n' "$C_OK" "$C_0" "$desc"
  else
    printf '  %sFAIL%s %s\n' "$C_NG" "$C_0" "$desc"; FAILED=1
  fi
}
dex() { docker exec "$NAME" "$@"; }
dexb() { docker exec "$NAME" bash -lc "$*"; }

require_docker() {
  docker info >/dev/null 2>&1 || die "Docker daemon が動いていません。'open -a Docker' で起動してください"
}

cmd_build() {
  require_docker
  info "検証イメージをビルド ($PLATFORM)"
  docker build --platform "$PLATFORM" -t "$IMAGE" "$REPO_ROOT/deploy/test"
}

cmd_up() {
  require_docker
  docker image inspect "$IMAGE" >/dev/null 2>&1 || cmd_build
  if docker inspect "$NAME" >/dev/null 2>&1; then
    info "コンテナ $NAME は既に存在します。起動を確認"
    docker start "$NAME" >/dev/null
  else
    info "コンテナ $NAME を起動 (systemd を PID 1 にする)"
    docker run -d --name "$NAME" --platform "$PLATFORM" \
      --privileged --cgroupns=host \
      --tmpfs /run --tmpfs /run/lock --tmpfs /tmp:exec \
      -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
      --stop-signal SIGRTMIN+3 \
      "${1:-$IMAGE}" /sbin/init >/dev/null
  fi
  info "systemd の起動を待機"
  dex systemctl is-system-running --wait >/dev/null 2>&1 || true
  dex systemctl is-system-running || true
}

cmd_sync() {
  info "作業ツリーを /opt/works/src へ流し込む"
  dex mkdir -p /opt/works/src
  # tar を root で展開するとアーカイブ側の uid が復元され、/opt/works/src は非 root 所有になる。
  # これは意図的に残す。git の safe.directory を踏む厳しめの条件で CD を検証できるため。
  # .git は含める（nix の git fetcher が必要）。node_modules と pb_data は除外。
  # COPYFILE_DISABLE=1 は macOS の tar が ._* を作るのを防ぐ。
  COPYFILE_DISABLE=1 tar --no-xattrs -C "$REPO_ROOT" -cf - \
      --exclude='./web/node_modules' \
      --exclude='./server/pb_data' \
      --exclude='./server/result-default' \
      --exclude='./.direnv' \
      --exclude='./result' --exclude='./result-*' \
      . \
    | docker exec -i "$NAME" tar -xf - -C /opt/works/src
  dexb 'git config --global --add safe.directory /opt/works/src'
}

cmd_dryrun() {
  info "dry-run（副作用が無いことを確認）"
  dex bash /opt/works/src/deploy/install.sh \
    --skip-clone --skip-tunnel --dry-run -y \
    --admin-email "$ADMIN_EMAIL" --generate-admin-password
  echo "--- dry-run 後の状態 ---"
  check "/nix が作られていない"      dexb '! test -d /nix'
  check "works ユーザーが作られていない" dexb '! id works'
  check "unit が置かれていない"      dexb '! test -f /etc/systemd/system/works-server.service'
  [ "$FAILED" = 0 ] || die "dry-run に副作用があります"
}

cmd_install() {
  info "install.sh を実行（初回は nix build に 15-30 分）"
  dex bash /opt/works/src/deploy/install.sh \
    --skip-clone --skip-tunnel -y \
    --admin-email "$ADMIN_EMAIL" --generate-admin-password \
    --event-name '検証イベント' --event-slug "$EVENT_SLUG" \
    --event-passphrase "$EVENT_PASSPHRASE"
}

cmd_verify() {
  info "検証アサーション"
  check "works-server が active"            dexb 'systemctl is-active --quiet works-server'
  check "/api/health が 200"                dexb 'curl -fsS --max-time 5 http://127.0.0.1:8090/api/health'
  check "SPA が配信される"                  dexb 'curl -fsS --max-time 5 http://127.0.0.1:8090/ | grep -qi "<div id=\"app\"\|<div id=app\|<script"'
  check "pb_data が works 所有"             dexb '[ "$(stat -c %U:%G /var/lib/works/pb_data)" = "works:works" ]'
  check "pb_data に root 所有ファイルが無い" dexb '[ -z "$(find /var/lib/works/pb_data ! -user works -print -quit)" ]'
  check "current が /nix/store を指す"      dexb 'readlink -f /opt/works/current | grep -q "^/nix/store/"'
  check "releases が GC ルートになっている"  dexb 'ls -1 /opt/works/releases | grep -q .'
  check "ffmpeg が wrapper 経由で引ける"     dexb '/opt/works/current/bin/works-server --help >/dev/null && grep -q ffmpeg /opt/works/current/bin/works-server'
  check "合言葉なしは 401"                  dexb '[ "$(curl -s -o /dev/null -w %{http_code} http://127.0.0.1:8090/api/collections/works/records)" = 401 ]'
  check "合言葉ありは 200（events 投入済み）" dexb "[ \"\$(curl -s -o /dev/null -w %{http_code} -H 'X-Event-Key: $EVENT_PASSPHRASE' http://127.0.0.1:8090/api/collections/works/records)\" = 200 ]"
  check "8090 が 127.0.0.1 のみで待受"       dexb 'ss -ltnp 2>/dev/null | grep ":8090" | grep -q "127.0.0.1"'
  [ "$FAILED" = 0 ] || die "検証に失敗した項目があります"
  info "全項目 PASS"
}

cmd_idempotent() {
  info "冪等性: 2回目の install.sh で再起動が起きないこと"
  local t1 t2
  t1=$(dex systemctl show -p ExecMainStartTimestamp --value works-server)
  dex bash /opt/works/src/deploy/install.sh \
    --skip-clone --skip-tunnel -y \
    --admin-email "$ADMIN_EMAIL" --generate-admin-password
  t2=$(dex systemctl show -p ExecMainStartTimestamp --value works-server)
  if [ "$t1" = "$t2" ]; then
    printf '  %sPASS%s 再起動なし (%s)\n' "$C_OK" "$C_0" "$t1"
  else
    printf '  %sFAIL%s 再起動が起きた: %s -> %s\n' "$C_NG" "$C_0" "$t1" "$t2"; FAILED=1
  fi
  check "2回目でも health が 200" dexb 'curl -fsS --max-time 5 http://127.0.0.1:8090/api/health'
  [ "$FAILED" = 0 ] || die "冪等性の検証に失敗しました"
}

cmd_backup() {
  info "backup.sh の検証"
  dex bash /opt/works/src/deploy/backup.sh --keep 3
  check "backups に zip ができた" dexb 'ls /var/lib/works/pb_data/backups/*.zip'
  check "zip が works 所有"       dexb '[ -z "$(find /var/lib/works/pb_data/backups ! -user works -print -quit)" ]'
  info "cold バックアップ"
  dex bash /opt/works/src/deploy/backup.sh --cold --out-dir /root/backups
  check "cold の tar ができた"    dexb 'ls /root/backups/*.tar.gz'
  check "cold 後にサービスが戻った" dexb 'systemctl is-active --quiet works-server'
  [ "$FAILED" = 0 ] || die "backup の検証に失敗しました"
}

# update.sh は origin から fetch する。GitHub の main には検証中の変更が無いので、
# コンテナ内にローカルの bare リポジトリを立てて origin にする。
cmd_origin() {
  info "ローカル origin を用意して update.sh を実地で検証できるようにする"
  dexb 'git config --global --add safe.directory "*"'
  dexb 'cd /opt/works/src \
    && git config user.email test@example.com && git config user.name test \
    && git add -A && (git diff --cached --quiet || git commit -q -m "test: 検証用スナップショット") \
    && rm -rf /opt/works/origin.git \
    && git clone -q --bare /opt/works/src /opt/works/origin.git \
    && git remote set-url origin /opt/works/origin.git'
  ok "origin -> /opt/works/origin.git"
}

cmd_update() {
  info "update.sh: 新しいコミットへ追従すること"
  dex bash /opt/works/src/deploy/update.sh --skip-backup
  check "追従後に health が 200" dexb 'curl -fsS --max-time 10 http://127.0.0.1:8090/api/health'

  info "update.sh: 変更が無ければ何もしないこと"
  local t1 t2 out
  t1=$(dex systemctl show -p ExecMainStartTimestamp --value works-server)
  out=$(dex bash /opt/works/src/deploy/update.sh --skip-backup 2>&1) || { echo "$out"; die "update.sh が失敗しました"; }
  if echo "$out" | grep -q 'すでに最新です'; then
    printf '  %sPASS%s 変更なしを検出して終了\n' "$C_OK" "$C_0"
  else
    printf '  %sFAIL%s 変更なし判定が働かなかった\n' "$C_NG" "$C_0"; FAILED=1
  fi
  t2=$(dex systemctl show -p ExecMainStartTimestamp --value works-server)
  if [ "$t1" = "$t2" ]; then
    printf '  %sPASS%s 再起動なし\n' "$C_OK" "$C_0"
  else
    printf '  %sFAIL%s 再起動が起きた\n' "$C_NG" "$C_0"; FAILED=1
  fi

  info "update.sh: --force で再ビルドして切り替わること"
  dex bash /opt/works/src/deploy/update.sh --skip-backup --force
  check "update 後も health が 200" dexb 'curl -fsS --max-time 10 http://127.0.0.1:8090/api/health'
  [ "$FAILED" = 0 ] || die "update の検証に失敗しました"
}

# 切替後に health が通らないケースを実地で作り、update.sh のロールバック経路を通す。
# nix build だけをスタブに差し替えた複製を使う（ロールバック判定そのものは本物のコードが動く）。
cmd_rollback() {
  info "ロールバック試験: 壊れたリリースに切り替わった後、update.sh が自動で戻すこと"
  local good
  good=$(dex readlink /opt/works/current)
  # shellcheck disable=SC2016  # コンテナ内で評価させるため $ は展開しない
  dexb 'sed "s|^nix_run() .*|nix_run() { shift; out=\"\"; while [ \$# -gt 0 ]; do [ \"\$1\" = -o ] \&\& { out=\$2; shift; }; shift; done; mkdir -p /opt/works/releases/.broken/bin; ln -sf /bin/false /opt/works/releases/.broken/bin/works-server; ln -sfn /opt/works/releases/.broken \"\$out\"; }|" \
      /opt/works/src/deploy/update.sh > /opt/works/update-broken.sh'
  # 新しいコミットを作って「更新がある」状態にする
  dexb 'cd /opt/works/src && date -u > .rollback-test && git add -A \
        && git commit -q -m "test: ロールバック検証用" \
        && git push -q origin HEAD:main'
  set +e
  dex bash /opt/works/update-broken.sh --skip-backup 2>&1 | tail -12
  set -e
  sleep 3
  local now
  now=$(dex readlink /opt/works/current)
  if [ "$now" = "$good" ]; then
    printf '  %sPASS%s current が元のリリースへ戻った\n' "$C_OK" "$C_0"
  else
    printf '  %sFAIL%s current=%s (期待 %s)\n' "$C_NG" "$C_0" "$now" "$good"; FAILED=1
  fi
  check "ロールバック後に health が 200" dexb 'curl -fsS --max-time 15 http://127.0.0.1:8090/api/health'
  dexb 'rm -rf /opt/works/releases/.broken /opt/works/update-broken.sh'
  dexb 'cd /opt/works/src && git rm -q --cached .rollback-test >/dev/null 2>&1; rm -f .rollback-test; git commit -q -am "test: 後始末" >/dev/null 2>&1; git push -q origin HEAD:main' || true
  [ "$FAILED" = 0 ] || die "ロールバック試験に失敗しました"
}

# docker commit は既定でコンテナを一時停止する。/nix 込みで数 GB あるため数分かかる。
# CD タイマーの検証。最重要は「変更が無いときにバックアップを作らないこと」。
# タイマーは10分ごとに回るので、ここが漏れると1日144個のバックアップが積まれる。
cmd_cd() {
  info "CD: release branch と自動更新タイマー"
  # sync は .git/config ごと流し込むので origin が GitHub に戻る。必要なら張り直す。
  dexb 'git -C /opt/works/src remote get-url origin | grep -q "^/opt/works/origin.git$"' || cmd_origin
  # ハーネスでは前回実行の release ref が残るので force で揃える。
  # 本番の CI は fast-forward のみ（main の書き換えを検知するため）。
  dexb 'cd /opt/works/src && git branch -D release >/dev/null 2>&1; \
        git push -q -f origin HEAD:release && git branch -f release HEAD'
  dexb 'sed -i "s|^WORKS_REF=.*|WORKS_REF=release|" /etc/works/install.env'
  dex bash /opt/works/src/deploy/install.sh --skip-clone --skip-tunnel -y \
      --admin-email admin@example.com --generate-admin-password --enable-cd
  check "works-update.timer が有効"  dexb 'systemctl is-enabled --quiet works-update.timer'
  check "works-update.timer が稼働"  dexb 'systemctl is-active --quiet works-update.timer'

  info "変更が無いとき、バックアップを作らずに抜けること"
  local before after
  before=$(dex bash -lc 'ls -1 /var/lib/works/pb_data/backups/ 2>/dev/null | wc -l' | tr -d ' ')
  dex systemctl start works-update.service
  after=$(dex bash -lc 'ls -1 /var/lib/works/pb_data/backups/ 2>/dev/null | wc -l' | tr -d ' ')
  if [ "$before" = "$after" ]; then
    printf '  %sPASS%s バックアップが増えていない (%s個のまま)\n' "$C_OK" "$C_0" "$before"
  else
    printf '  %sFAIL%s バックアップが %s -> %s に増えた\n' "$C_NG" "$C_0" "$before" "$after"; FAILED=1
  fi
  check "更新サービスが正常終了" dexb 'systemctl show -p Result --value works-update.service | grep -q success'
  check "更新後も health が 200" dexb 'curl -fsS --max-time 10 http://127.0.0.1:8090/api/health'

  info "止められること"
  dex systemctl disable --now works-update.timer
  check "タイマーを止められた" dexb '! systemctl is-active --quiet works-update.timer'
  [ "$FAILED" = 0 ] || die "CD の検証に失敗しました"
}

cmd_snapshot() {
  info "スナップショット $SNAPSHOT を作成（/nix 込みで数分かかります）"
  docker commit "$NAME" "$SNAPSHOT" >/dev/null
  ok "$SNAPSHOT（reset で復元できます）"
}
cmd_reset()    { info "スナップショットからコンテナを作り直す"; cmd_down; cmd_up "$SNAPSHOT"; }
cmd_shell()    { docker exec -it "$NAME" bash; }
cmd_logs()     { dex journalctl -u works-server -n "${1:-80}" --no-pager; }
cmd_down()     { docker rm -f "$NAME" >/dev/null 2>&1 || true; info "コンテナを破棄"; }

cmd_all() {
  cmd_up; cmd_sync; cmd_dryrun; cmd_install; cmd_verify
  cmd_idempotent; cmd_backup; cmd_origin; cmd_update; cmd_rollback; cmd_cd
  info "一気通貫の検証が完了しました"
  info "反復するなら 'snapshot' でイメージを保存し、'reset' で復元できます"
}

[ $# -gt 0 ] || { grep -E '^cmd_[a-z]+\(\)' "$0" | sed 's/^cmd_/  /;s/().*//' >&2; die "サブコマンドを指定してください"; }
for sub in "$@"; do
  case $sub in
    build|up|sync|dryrun|install|verify|idempotent|backup|origin|update|rollback|cd|snapshot|reset|shell|logs|down|all) "cmd_$sub" ;;
    *) die "未知のサブコマンド: $sub" ;;
  esac
done
