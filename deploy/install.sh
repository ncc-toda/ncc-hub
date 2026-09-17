#!/usr/bin/env bash
# NccHub ワンショットインストーラ（Ubuntu / Debian）
#
#   sudo bash install.sh --admin-email <mail> --generate-admin-password \
#                        --tunnel-token-file /path/to/token.txt
#
# 再実行しても安全（冪等）。詳細は docs/deployment.md を参照。
# 注意: 単体で curl | bash できるよう、他ファイルを source しない自己完結スクリプトにしている。
#       log/die/wait_health まわりは update.sh / backup.sh と重複する。変更時は 3 本を揃えること。
set -Eeuo pipefail

# ---------------------------------------------------------------- 既定値
REPO=https://github.com/ncc-toda/ncc-hub.git
REF=main
PREFIX=/opt/works
DATA_DIR=/var/lib/works
SVC_USER=works
LISTEN=127.0.0.1:8090
KEEP_RELEASES=3
NIX_INSTALL_URL=https://nixos.org/nix/install
TUNNEL_PROTOCOL=auto
SECRETS_FILE=/root/ncc-hub-install-secrets.txt
BACKUP_ENV=/etc/works/backup.env
BACKUP_EMAIL=backup@ncc-hub.local
STATE_DIR=/etc/works/state

ADMIN_EMAIL=${WORKS_ADMIN_EMAIL:-}
ADMIN_PASSWORD=${WORKS_ADMIN_PASSWORD:-}
ADMIN_PASSWORD_FILE=${WORKS_ADMIN_PASSWORD_FILE:-}
GEN_ADMIN_PASSWORD=0
FORCE_ADMIN_PASSWORD=0
TUNNEL_TOKEN=${WORKS_TUNNEL_TOKEN:-}
TUNNEL_TOKEN_FILE=${WORKS_TUNNEL_TOKEN_FILE:-}
EVENT_NAME=; EVENT_SLUG=; EVENT_PASSPHRASE=; EVENT_PASSPHRASE_FILE=
SKIP_NIX=0; SKIP_TUNNEL=0; SKIP_CLONE=0
DRY_RUN=0; NON_INTERACTIVE=0; VERBOSE=0

# ---------------------------------------------------------------- 出力
C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_ERR=$'\033[31m'; C_IN=$'\033[36m'; C_0=$'\033[0m'
info() { printf '%s==>%s %s\n' "$C_IN" "$C_0" "$*"; }
ok()   { printf '  %s✓%s %s\n' "$C_OK" "$C_0" "$*"; }
warn() { printf '%s警告:%s %s\n' "$C_WARN" "$C_0" "$*" >&2; }
die()  { printf '%sエラー:%s %s\n' "$C_ERR" "$C_0" "$*" >&2; exit 1; }
dbg()  { [ "$VERBOSE" = 1 ] && printf '    %s\n' "$*" >&2 || true; }

# 書き込み系はこれを通す。dry-run では表示のみ。読み取り系は直接呼ぶ。
run() {
  if [ "$DRY_RUN" = 1 ]; then printf '  [dry-run] %s\n' "$*"; return 0; fi
  dbg "$*"; "$@"
}
# パイプやリダイレクトを含む一括実行用。
runsh() {
  if [ "$DRY_RUN" = 1 ]; then printf '  [dry-run] sh -c %s\n' "$1"; return 0; fi
  dbg "sh -c $1"; bash -c "$1"
}

usage() {
  cat <<'USAGE'
NccHub ワンショットインストーラ（Ubuntu / Debian）

  sudo bash install.sh --admin-email <mail> --generate-admin-password \
                       --tunnel-token-file /path/to/token.txt

再実行しても安全（冪等）。詳細は docs/deployment.md を参照。

主なオプション:
  --admin-email EMAIL              管理者(superuser)のメール
  --admin-password-file PATH       管理者パスワードをファイルから読む（推奨）
  --admin-password PW              管理者パスワードを直接指定（ps に露出する）
  --generate-admin-password        32文字のパスワードを自動生成する
  --force-admin-password           既存 superuser のパスワードを上書きする
  --tunnel-token-file PATH         Cloudflare Tunnel の token をファイルから読む（推奨）
  --tunnel-token TOKEN             token を直接指定（ps に露出する）
  --tunnel-protocol auto|http2|quic  UDP/7844 が塞がれている環境では http2
  --skip-tunnel                    トンネル設定を行わない
  --event-name NAME                初期イベント名
  --event-slug SLUG                初期イベントの slug
  --event-passphrase-file PATH     初期イベントの合言葉をファイルから読む
  --event-passphrase PW            初期イベントの合言葉を直接指定
  --repo URL                       既定 https://github.com/ncc-toda/ncc-hub.git
  --ref REF                        既定 main
  --skip-clone                     既存の $PREFIX/src をそのまま使う
  --skip-nix                       Nix の導入を行わない（導入済み前提）
  --nix-install-url URL            Nix インストーラの URL
  --prefix DIR                     既定 /opt/works
  --data-dir DIR                   既定 /var/lib/works
  --user NAME                      既定 works
  --listen ADDR                    既定 127.0.0.1:8090
  --keep-releases N                保持するリリース世代数（既定 3）
  --dry-run                        実行せず計画のみ表示
  -y, --non-interactive            対話プロンプトを出さない
  -v, --verbose                    実行コマンドを表示
  -h, --help                       このヘルプ
USAGE
}

# ---------------------------------------------------------------- 引数
parse_args() {
  while [ $# -gt 0 ]; do
    case $1 in
      --admin-email)           ADMIN_EMAIL=$2; shift 2 ;;
      --admin-password)        ADMIN_PASSWORD=$2; shift 2 ;;
      --admin-password-file)   ADMIN_PASSWORD_FILE=$2; shift 2 ;;
      --generate-admin-password) GEN_ADMIN_PASSWORD=1; shift ;;
      --force-admin-password)  FORCE_ADMIN_PASSWORD=1; shift ;;
      --tunnel-token)          TUNNEL_TOKEN=$2; shift 2 ;;
      --tunnel-token-file)     TUNNEL_TOKEN_FILE=$2; shift 2 ;;
      --tunnel-protocol)       TUNNEL_PROTOCOL=$2; shift 2 ;;
      --skip-tunnel)           SKIP_TUNNEL=1; shift ;;
      --event-name)            EVENT_NAME=$2; shift 2 ;;
      --event-slug)            EVENT_SLUG=$2; shift 2 ;;
      --event-passphrase)      EVENT_PASSPHRASE=$2; shift 2 ;;
      --event-passphrase-file) EVENT_PASSPHRASE_FILE=$2; shift 2 ;;
      --repo)                  REPO=$2; shift 2 ;;
      --ref)                   REF=$2; shift 2 ;;
      --skip-clone)            SKIP_CLONE=1; shift ;;
      --skip-nix)              SKIP_NIX=1; shift ;;
      --nix-install-url)       NIX_INSTALL_URL=$2; shift 2 ;;
      --prefix)                PREFIX=$2; shift 2 ;;
      --data-dir)              DATA_DIR=$2; shift 2 ;;
      --user)                  SVC_USER=$2; shift 2 ;;
      --listen)                LISTEN=$2; shift 2 ;;
      --keep-releases)         KEEP_RELEASES=$2; shift 2 ;;
      --dry-run)               DRY_RUN=1; shift ;;
      -y|--non-interactive)    NON_INTERACTIVE=1; shift ;;
      -v|--verbose)            VERBOSE=1; shift ;;
      -h|--help)               usage; exit 0 ;;
      *) die "未知のオプション: $1（--help でヘルプ）" ;;
    esac
  done
  SRC_DIR=$PREFIX/src
  PB_DATA=$DATA_DIR/pb_data
  UNIT=/etc/systemd/system/works-server.service
}

prompt() { # prompt <表示> <変数名> [secret]
  local msg=$1 var=$2 secret=${3:-} val=
  [ "$NON_INTERACTIVE" = 1 ] && return 1
  [ -t 0 ] || return 1
  if [ -n "$secret" ]; then read -r -s -p "$msg: " val; echo; else read -r -p "$msg: " val; fi
  [ -n "$val" ] || return 1
  printf -v "$var" '%s' "$val"
}

read_secret_file() { # read_secret_file <path>
  [ -f "$1" ] || die "ファイルが見つかりません: $1"
  # 末尾改行のみ落とす。パスワードに空白が含まれうるので trim はしない。
  printf '%s' "$(cat "$1")"
}

# /tmp を noexec でマウントしている環境があるため、実行可能な作業ディレクトリを自前で選ぶ。
# Nix のインストーラは tarball を展開して実行するので、ここを外すと Permission denied になる。
pick_workdir() {
  local d probe
  for d in "${TMPDIR:-/tmp}" /var/tmp "$PREFIX"; do
    [ -d "$d" ] || continue
    probe=$(mktemp -d "$d/ncc-hub-install.XXXXXX" 2>/dev/null) || continue
    printf '#!/bin/sh\nexit 0\n' > "$probe/probe.sh"
    chmod +x "$probe/probe.sh"
    if "$probe/probe.sh" >/dev/null 2>&1; then
      WORKDIR=$probe
      [ "$d" = "${TMPDIR:-/tmp}" ] || warn "${TMPDIR:-/tmp} が noexec のため作業ディレクトリに $d を使います"
      return 0
    fi
    rm -rf "$probe"
  done
  die "実行可能な一時ディレクトリが見つかりません（/tmp と /var/tmp が noexec です）"
}

cleanup_workdir() { [ -n "${WORKDIR:-}" ] && rm -rf "$WORKDIR" || true; }

gen_password() {
  # tr <urandom | head は head の早期終了で SIGPIPE になり、pipefail で落ちる。
  # 先に固定長を読み切ってから絞る。512 バイトあれば英数字は 100 文字以上残る。
  local raw
  raw=$(head -c 512 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9')
  [ ${#raw} -ge 32 ] || die "乱数の生成に失敗しました"
  printf '%s' "${raw:0:32}"
}

resolve_secrets() {
  [ -n "$ADMIN_PASSWORD_FILE" ] && ADMIN_PASSWORD=$(read_secret_file "$ADMIN_PASSWORD_FILE")
  [ -n "$TUNNEL_TOKEN_FILE" ]   && TUNNEL_TOKEN=$(read_secret_file "$TUNNEL_TOKEN_FILE")
  [ -n "$EVENT_PASSPHRASE_FILE" ] && EVENT_PASSPHRASE=$(read_secret_file "$EVENT_PASSPHRASE_FILE")

  [ -n "${WORKS_ADMIN_PASSWORD:-}" ] && [ -z "$ADMIN_PASSWORD_FILE" ] && warn "環境変数でパスワードを渡しています。--admin-password-file を推奨します" || true
  # 直接指定は ps に載る。警告のみで続行する。
  case " $ORIG_ARGS " in *" --admin-password "*) warn "--admin-password は ps に露出します。--admin-password-file を推奨します";; esac
  case " $ORIG_ARGS " in *" --tunnel-token "*)  warn "--tunnel-token は ps に露出します。--tunnel-token-file を推奨します";; esac

  if [ -z "$ADMIN_EMAIL" ]; then
    prompt "管理者(先生)のメールアドレス" ADMIN_EMAIL \
      || die "--admin-email が必要です"
  fi
  case $ADMIN_EMAIL in *@*.*) : ;; *) die "メールアドレスの形式が不正です: $ADMIN_EMAIL" ;; esac

  if [ -z "$ADMIN_PASSWORD" ]; then
    if [ "$GEN_ADMIN_PASSWORD" = 1 ]; then
      ADMIN_PASSWORD=$(gen_password); ADMIN_PASSWORD_GENERATED=1
    else
      prompt "管理者パスワード(20文字以上)" ADMIN_PASSWORD secret \
        || die "--admin-password-file か --generate-admin-password が必要です"
    fi
  fi
  [ ${#ADMIN_PASSWORD} -ge 20 ] || die "管理者パスワードは 20 文字以上にしてください（現在 ${#ADMIN_PASSWORD} 文字）"

  if [ -n "$EVENT_SLUG" ]; then
    [ -n "$EVENT_PASSPHRASE" ] || die "--event-slug を指定する場合は合言葉も必要です"
    [ ${#EVENT_PASSPHRASE} -ge 8 ] || die "合言葉は 8 文字以上にしてください"
    [ -n "$EVENT_NAME" ] || EVENT_NAME=$EVENT_SLUG
    case $EVENT_SLUG in *[!a-z0-9-]*) die "--event-slug は半角英小文字・数字・ハイフンのみです: $EVENT_SLUG";; esac
  fi

  if [ "$SKIP_TUNNEL" = 0 ] && [ -z "$TUNNEL_TOKEN" ]; then
    warn "Tunnel token が指定されていません。トンネル設定をスキップします（後から再実行で追加できます）"
    SKIP_TUNNEL=1
  fi
  case $TUNNEL_PROTOCOL in auto|http2|quic) : ;; *) die "--tunnel-protocol は auto|http2|quic のいずれかです" ;; esac
}

# ---------------------------------------------------------------- 前提検査
preflight() {
  info "前提検査"
  [ "$(id -u)" = 0 ] || die "root で実行してください（sudo bash $0 ...）"
  [ -d /run/systemd/system ] || die "systemd が PID 1 で動いていません。コンテナなら --privileged と /sbin/init が必要です"
  command -v apt-get >/dev/null || die "Ubuntu / Debian 以外には未対応です"

  local id_like=""
  # shellcheck source=/dev/null disable=SC1091
  [ -r /etc/os-release ] && . /etc/os-release && id_like="${ID:-}"
  case $id_like in ubuntu|debian) ok "OS: ${PRETTY_NAME:-$id_like}" ;;
    *) warn "想定外の OS です（ID=$id_like）。続行しますが未検証です" ;;
  esac

  local arch; arch=$(dpkg --print-architecture)
  case $arch in amd64|arm64) ok "アーキテクチャ: $arch" ;; *) die "未対応のアーキテクチャです: $arch" ;; esac

  # /nix 側と データ側の空き。SPEC §16 の「一時的に最大 ~5GB」を踏まえた要求値。
  local nix_free data_free
  nix_free=$(df -Pk /nix 2>/dev/null || df -Pk /); nix_free=$(echo "$nix_free" | awk 'NR==2{print int($4/1048576)}')
  run mkdir -p "$DATA_DIR"
  data_free=$(df -Pk "$DATA_DIR" 2>/dev/null || df -Pk /); data_free=$(echo "$data_free" | awk 'NR==2{print int($4/1048576)}')
  [ "${nix_free:-0}"  -ge 20 ] || warn "/nix 側の空きが ${nix_free}GB です。20GB 以上を推奨します"
  [ "${data_free:-0}" -ge 10 ] || warn "$DATA_DIR の空きが ${data_free}GB です。2GB 動画の変換には一時的に約 5GB 使います"
  ok "空き容量: /nix ${nix_free}GB, データ ${data_free}GB"

  local mem_mb swap_mb total_mb
  mem_mb=$(awk '/MemTotal/{print int($2/1024)}' /proc/meminfo)
  swap_mb=$(awk '/SwapTotal/{print int($2/1024)}' /proc/meminfo)
  total_mb=$((mem_mb + swap_mb))
  [ "$total_mb" -ge 4000 ] || warn "メモリ+swap が ${total_mb}MB です。4GB 未満だと Go のビルドが OOM で落ちることがあります"
  ok "メモリ+swap: ${total_mb}MB"

  local host
  for host in cache.nixos.org github.com; do
    if curl -fsS --max-time 10 -o /dev/null "https://$host/" 2>/dev/null; then
      ok "疎通: $host"
    else
      warn "https://$host へ到達できません。プロキシ設定を確認してください"
    fi
  done

  if command -v timedatectl >/dev/null 2>&1; then
    if timedatectl show -p NTPSynchronized --value 2>/dev/null | grep -q yes; then
      ok "時刻同期: OK"
    else
      warn "時刻が同期されていません。TLS やトンネルが失敗することがあります"
    fi
  fi
}

apt_bootstrap() {
  info "基本パッケージを確認"
  local need=() p
  for p in curl ca-certificates git xz-utils jq; do
    dpkg -s "$p" >/dev/null 2>&1 || need+=("$p")
  done
  # runuser は util-linux。Ubuntu/Debian では標準で入っている。
  command -v runuser >/dev/null 2>&1 || need+=(util-linux)
  if [ ${#need[@]} -gt 0 ]; then
    run apt-get update -qq
    run apt-get install -y --no-install-recommends "${need[@]}"
    ok "導入: ${need[*]}"
  else
    ok "基本パッケージは導入済み"
  fi
}

# ---------------------------------------------------------------- Nix
source_nix_profile() {
  local p=/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
  # nix のプロファイルスクリプトは未定義変数を参照するため、一時的に -u を外す。
  # shellcheck source=/dev/null
  if [ -r "$p" ]; then set +u; . "$p"; set -u; fi
}

ensure_nix() {
  if command -v nix >/dev/null 2>&1 || [ -x /nix/var/nix/profiles/default/bin/nix ]; then
    ok "Nix は導入済み"
  elif [ "$SKIP_NIX" = 1 ]; then
    die "--skip-nix が指定されていますが nix が見つかりません"
  else
    info "Nix (multi-user) を導入"
    local tmp="$WORKDIR/nix"
    run mkdir -p "$tmp"
    run curl -fsSL -o "$tmp/nix-install.sh" "$NIX_INSTALL_URL"
    if [ "$DRY_RUN" = 0 ]; then
      printf 'experimental-features = nix-command flakes\n' > "$tmp/nix-extra.conf"
    fi
    # インストーラは tarball を TMPDIR へ展開して実行する。noexec な /tmp を避けるため明示する。
    # --no-channel-add: flake.lock でピンしているのでチャンネルは不要（時間とディスクの節約）
    run env TMPDIR="$tmp" sh "$tmp/nix-install.sh" --daemon --yes --no-channel-add \
        --nix-extra-conf-file "$tmp/nix-extra.conf"
    ok "Nix を導入しました"
  fi

  source_nix_profile
  NIX_BIN=$(command -v nix 2>/dev/null || echo /nix/var/nix/profiles/default/bin/nix)
  [ "$DRY_RUN" = 1 ] || [ -x "$NIX_BIN" ] || die "nix が PATH に見つかりません: $NIX_BIN"

  if [ "$DRY_RUN" = 0 ] && ! systemctl is-active --quiet nix-daemon; then
    run systemctl enable --now nix-daemon
  fi
  ensure_flakes_conf
}

# nix.conf への追記は人間が手で nix を叩くときの利便性のため。
# スクリプト内の nix 呼び出しは常に --extra-experimental-features を付けるので、
# ここが失敗しても install は成立する。
ensure_flakes_conf() {
  local conf=/etc/nix/nix.conf
  [ "$DRY_RUN" = 1 ] && { printf '  [dry-run] %s に experimental-features を確認\n' "$conf"; return 0; }
  mkdir -p "$(dirname "$conf")"; touch "$conf"
  if grep -qE '^[[:space:]]*experimental-features' "$conf"; then
    grep -qE '^[[:space:]]*experimental-features.*flakes' "$conf" \
      || warn "$conf の experimental-features に flakes がありません。手動実行時は --extra-experimental-features を付けてください"
  else
    printf 'experimental-features = nix-command flakes\n' >> "$conf"
    systemctl is-active --quiet nix-daemon && systemctl restart nix-daemon || true
    ok "$conf に flakes を有効化しました"
  fi
}

nix_run() { run "$NIX_BIN" --extra-experimental-features "nix-command flakes" "$@"; }

# ---------------------------------------------------------------- ユーザーとディレクトリ
ensure_user() {
  info "サービスユーザーとディレクトリ"
  getent group "$SVC_USER" >/dev/null || run groupadd --system "$SVC_USER"
  if ! id -u "$SVC_USER" >/dev/null 2>&1; then
    run useradd --system --gid "$SVC_USER" --home-dir "$DATA_DIR" \
        --shell /usr/sbin/nologin "$SVC_USER"
    ok "ユーザー $SVC_USER を作成"
  else
    ok "ユーザー $SVC_USER は作成済み"
  fi

  run install -d -o "$SVC_USER" -g "$SVC_USER" -m 0750 "$DATA_DIR" "$PB_DATA"
  run install -d -m 0755 "$PREFIX" "$PREFIX/releases"
  run install -d -m 0750 /etc/works
  run install -d -m 0700 "$STATE_DIR"
  # 旧版は pb_data 配下にマーカーを置いていた。バックアップ対象に混ざるので移設する。
  if [ "$DRY_RUN" = 0 ] && [ -d "$PB_DATA/.works-install" ]; then
    cp -a "$PB_DATA/.works-install/." "$STATE_DIR/" 2>/dev/null || true
    rm -rf "$PB_DATA/.works-install"
  fi
  # 既存データには触らない。空のときだけ所有者を揃える。
  if [ "$DRY_RUN" = 0 ] && [ -z "$(ls -A "$PB_DATA" 2>/dev/null)" ]; then
    chown -R "$SVC_USER:$SVC_USER" "$DATA_DIR"
  fi
  ok "$PB_DATA を用意"
}

# ---------------------------------------------------------------- ソース取得
sync_source() {
  if [ "$SKIP_CLONE" = 1 ]; then
    [ -d "$SRC_DIR" ] || die "--skip-clone が指定されていますが $SRC_DIR がありません"
    ok "既存のソースを使用: $SRC_DIR"
  elif [ -d "$SRC_DIR/.git" ]; then
    info "ソースを更新: $REF"
    run git -C "$SRC_DIR" remote set-url origin "$REPO"
    # shallow clone は nix の git fetcher が revision を解決できないため使わない。
    run git -C "$SRC_DIR" fetch --prune origin "$REF"
    if [ "$DRY_RUN" = 0 ] && [ -n "$(git -C "$SRC_DIR" status --porcelain)" ]; then
      warn "$SRC_DIR に未コミットの変更があります。reset --hard で破棄します"
    fi
    run git -C "$SRC_DIR" reset --hard FETCH_HEAD
  elif [ -e "$SRC_DIR" ] && [ -n "$(ls -A "$SRC_DIR" 2>/dev/null)" ]; then
    die "$SRC_DIR が git リポジトリでない状態で存在します。退避してから再実行してください"
  else
    info "ソースを取得: $REPO ($REF)"
    run git clone --branch "$REF" "$REPO" "$SRC_DIR"
  fi
  run git config --global --add safe.directory "$SRC_DIR"
}

# ---------------------------------------------------------------- ビルドと切替
build_release() {
  info "ビルド（初回は 15〜30 分かかります）"
  if [ "$DRY_RUN" = 1 ]; then
    REL="<dry-run>"; printf '  [dry-run] nix build %s#default -o %s/releases/<sha>\n' "$SRC_DIR" "$PREFIX"; return 0
  fi
  REL=$(git -C "$SRC_DIR" rev-parse --short=12 HEAD)
  if [ -n "$(git -C "$SRC_DIR" status --porcelain)" ]; then
    # 作業ツリーが dirty なときは内容のハッシュを付ける。
    # タイムスタンプにすると同じ内容でも毎回別リリース扱いになり、冪等でなくなる。
    local dirty
    dirty=$({ git -C "$SRC_DIR" diff HEAD --binary
              git -C "$SRC_DIR" ls-files --others --exclude-standard; } \
            | sha256sum | cut -c1-8)
    REL="${REL}-dirty-${dirty}"
  fi
  # releases/<sha> 自体を out-link（= GC ルート）にする。
  # current を直接 out-link にすると、切替時に旧世代の GC ルートが消えてロールバック先を失う。
  nix_run build "$SRC_DIR#default" -o "$PREFIX/releases/$REL" --print-build-logs
  ok "リリース $REL をビルド"
}

activate_release() {
  [ "$DRY_RUN" = 1 ] && { printf '  [dry-run] %s/current を切り替え\n' "$PREFIX"; return 0; }
  PREV_TARGET=$(readlink "$PREFIX/current" 2>/dev/null || true)
  local new="$PREFIX/releases/$REL"
  if [ "$PREV_TARGET" = "$new" ]; then
    ok "current は既に $REL を指しています"
    ACTIVATED=0; return 0
  fi
  # ln -sfn はディレクトリ symlink に対して unlink+symlink になり原子的でない。
  # mv -T は rename(2) なので原子的に差し替えられる。
  ln -sfn "$new" "$PREFIX/.current.tmp"
  mv -Tf "$PREFIX/.current.tmp" "$PREFIX/current"
  ACTIVATED=1
  ok "current -> $REL"
}

prune_releases() {
  [ "$DRY_RUN" = 1 ] && return 0
  local cur; cur=$(readlink "$PREFIX/current" 2>/dev/null || true)
  local n=0 r
  while IFS= read -r r; do
    [ -n "$r" ] || continue
    [ "$r" = "$cur" ] && continue
    n=$((n + 1))
    [ "$n" -lt "$KEEP_RELEASES" ] && continue
    rm -f "$r"
  done < <(find "$PREFIX/releases" -maxdepth 1 -mindepth 1 -type l -printf '%T@ %p\n' 2>/dev/null \
            | sort -rn | cut -d' ' -f2-)
}

# ---------------------------------------------------------------- superuser
ensure_superuser() {
  info "管理者アカウント"
  local marker="$STATE_DIR/superuser-created"
  if [ "$FORCE_ADMIN_PASSWORD" = 0 ] && [ -f "$marker" ]; then
    ok "管理者は作成済み（パスワードは変更しません）"
    return 0
  fi
  if [ "$DRY_RUN" = 1 ]; then
    printf '  [dry-run] works-server superuser create %s <password>\n' "$ADMIN_EMAIL"; return 0
  fi

  # root で実行すると pb_data に root 所有のファイルができ、以後サービスが書けなくなる。
  # runuser でサービスユーザーに落として実行するのは必須。
  local sub=create out rc
  [ "$FORCE_ADMIN_PASSWORD" = 1 ] && sub=upsert
  set +e
  out=$(runuser -u "$SVC_USER" -- "$PREFIX/current/bin/works-server" \
          superuser "$sub" "$ADMIN_EMAIL" "$ADMIN_PASSWORD" --dir="$PB_DATA" 2>&1)
  rc=$?
  set -e
  if [ $rc -ne 0 ]; then
    if printf '%s' "$out" | grep -qiE 'unique|already|exists'; then
      warn "管理者 $ADMIN_EMAIL は既に存在します。パスワードは変更しません（--force-admin-password で上書き可）"
      ADMIN_PASSWORD_GENERATED=0
    else
      die "管理者の作成に失敗しました: $out"
    fi
  else
    ok "管理者 $ADMIN_EMAIL を作成"
  fi
  install -d -m 0700 "$STATE_DIR"
  : > "$marker"
}

# バックアップ専用の superuser を作る。PocketBase は superuser の権限を分割できないので
# 特権の縮小にはならないが、先生本人のパスワードをディスクに置かずに済む。
ensure_backup_credentials() {
  local marker="$STATE_DIR/backup-user-created"
  if [ -f "$BACKUP_ENV" ] && [ -f "$marker" ]; then
    ok "バックアップ用の認証情報は作成済み"
    return 0
  fi
  if [ "$DRY_RUN" = 1 ]; then
    printf '  [dry-run] バックアップ用 superuser %s を作成し %s を書き出す\n' "$BACKUP_EMAIL" "$BACKUP_ENV"
    return 0
  fi
  info "バックアップ用の認証情報"
  local pass out rc
  pass=$(gen_password)
  set +e
  out=$(runuser -u "$SVC_USER" -- "$PREFIX/current/bin/works-server" \
          superuser upsert "$BACKUP_EMAIL" "$pass" --dir="$PB_DATA" 2>&1)
  rc=$?
  set -e
  [ $rc -eq 0 ] || die "バックアップ用 superuser の作成に失敗しました: $out"

  install -d -m 0700 "$STATE_DIR"
  umask 077
  cat > "$BACKUP_ENV" <<EOF
# deploy/backup.sh が使うバックアップ専用 superuser の認証情報。
# install.sh が生成し、再実行では作り直さない。
WORKS_BACKUP_EMAIL=$BACKUP_EMAIL
WORKS_BACKUP_PASSWORD=$pass
EOF
  chmod 600 "$BACKUP_ENV"
  : > "$marker"
  ok "$BACKUP_ENV を作成（0600）"
}

# ---------------------------------------------------------------- systemd
render_unit() {
  sed -e "s|127\.0\.0\.1:8090|$LISTEN|g" \
      -e "s|/var/lib/works|$DATA_DIR|g" \
      -e "s|/opt/works/current|$PREFIX/current|g" \
      -e "s|^User=works$|User=$SVC_USER|" \
      -e "s|^Group=works$|Group=$SVC_USER|" \
      "$SRC_DIR/deploy/works-server.service"
}

install_unit() {
  info "systemd unit"
  if [ "$DRY_RUN" = 1 ]; then printf '  [dry-run] %s を配置\n' "$UNIT"; return 0; fi
  local tmp; tmp=$(mktemp "$WORKDIR/unit.XXXXXX")
  render_unit > "$tmp"
  if ! cmp -s "$tmp" "$UNIT"; then
    install -m 0644 "$tmp" "$UNIT"
    systemctl daemon-reload
    UNIT_CHANGED=1
    ok "$UNIT を更新"
  else
    ok "$UNIT は最新"
  fi
  rm -f "$tmp"
  systemctl enable works-server >/dev/null 2>&1 || systemctl enable works-server
}

start_service() {
  [ "$DRY_RUN" = 1 ] && { printf '  [dry-run] works-server を起動\n'; return 0; }
  if [ "${UNIT_CHANGED:-0}" = 1 ] || [ "${ACTIVATED:-0}" = 1 ] || ! systemctl is-active --quiet works-server; then
    info "works-server を起動"
    systemctl restart works-server
  else
    ok "works-server は稼働中（再起動なし）"
  fi
}

dump_diagnostics() {
  printf '\n%s--- systemctl status ---%s\n' "$C_ERR" "$C_0" >&2
  systemctl status works-server --no-pager -l 2>&1 | head -30 >&2 || true
  printf '\n%s--- journalctl -u works-server ---%s\n' "$C_ERR" "$C_0" >&2
  journalctl -u works-server -n 120 --no-pager 2>&1 | tail -60 >&2 || true
}

wait_health() { # wait_health [timeout秒]
  [ "$DRY_RUN" = 1 ] && { printf '  [dry-run] /api/health を待機\n'; return 0; }
  local deadline=$(( $(date +%s) + ${1:-90} ))
  info "/api/health の応答を待機"
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if curl -fsS --max-time 3 "http://$LISTEN/api/health" >/dev/null 2>&1; then
      ok "health OK"; return 0
    fi
    # crash-loop なら 90 秒待たずに即失敗させる。
    systemctl is-active --quiet works-server || { warn "works-server が停止しました"; break; }
    sleep 1
  done
  dump_diagnostics
  return 1
}

# ---------------------------------------------------------------- events 初期投入
seed_event() {
  [ -n "$EVENT_SLUG" ] || return 0
  info "イベント $EVENT_SLUG を投入"
  if [ "$DRY_RUN" = 1 ]; then printf '  [dry-run] events に %s を作成\n' "$EVENT_SLUG"; return 0; fi
  local token count
  # 秘密情報は stdin で渡す。argv に載せない。
  token=$(jq -nc --arg i "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{identity:$i,password:$p}' \
    | curl -fsS --max-time 20 -X POST "http://$LISTEN/api/collections/_superusers/auth-with-password" \
        -H 'Content-Type: application/json' --data-binary @- 2>/dev/null \
    | jq -r '.token // empty') || true
  if [ -z "$token" ]; then
    warn "管理者として認証できませんでした。events の投入をスキップします（管理画面から作成してください）"
    return 0
  fi

  count=$(curl -fsS --max-time 20 -G "http://$LISTEN/api/collections/events/records" \
            --data-urlencode "filter=(slug='$EVENT_SLUG')" --data-urlencode "perPage=1" \
            -H "Authorization: $token" | jq -r '.totalItems // 0')
  if [ "$count" != 0 ]; then
    ok "イベント $EVENT_SLUG は既に存在します"
    return 0
  fi

  if jq -nc --arg n "$EVENT_NAME" --arg s "$EVENT_SLUG" --arg p "$EVENT_PASSPHRASE" \
       '{name:$n,slug:$s,passphrase:$p,max_video_bytes:2147483648,submissions_open:true}' \
      | curl -fsS --max-time 20 -X POST "http://$LISTEN/api/collections/events/records" \
          -H "Authorization: $token" -H 'Content-Type: application/json' --data-binary @- >/dev/null
  then
    ok "イベント $EVENT_SLUG を作成"
  else
    warn "イベントの作成に失敗しました。管理画面から作成してください"
  fi
}

# ---------------------------------------------------------------- cloudflared
install_cloudflared_deb_fallback() {
  local arch deb tmp
  arch=$(dpkg --print-architecture)
  tmp=$(mktemp -d "$WORKDIR/cfdeb.XXXXXX"); deb="$tmp/cloudflared.deb"
  warn "apt リポジトリからの導入に失敗しました。GitHub releases からの取得を試みます"
  run curl -fsSL -o "$deb" \
    "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${arch}.deb"
  run dpkg -i "$deb"
}

ensure_cloudflared() {
  [ "$SKIP_TUNNEL" = 1 ] && return 0
  info "cloudflared を導入"
  if dpkg -s cloudflared >/dev/null 2>&1 || command -v cloudflared >/dev/null 2>&1; then
    ok "cloudflared は導入済み（$(cloudflared --version 2>/dev/null | head -1)）"
    return 0
  fi
  if [ "$DRY_RUN" = 1 ]; then printf '  [dry-run] cloudflared を apt で導入\n'; return 0; fi

  install -d -m 0755 /usr/share/keyrings
  if curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg -o /usr/share/keyrings/cloudflare-main.gpg; then
    local list=/etc/apt/sources.list.d/cloudflared.list tmp; tmp=$(mktemp "$WORKDIR/cf.XXXXXX")
    printf 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main\n' > "$tmp"
    cmp -s "$tmp" "$list" || install -m 0644 "$tmp" "$list"
    rm -f "$tmp"
    if ! { apt-get update -qq && apt-get install -y --no-install-recommends cloudflared; }; then
      install_cloudflared_deb_fallback
    fi
  else
    install_cloudflared_deb_fallback
  fi
  ok "cloudflared を導入（$(cloudflared --version 2>/dev/null | head -1)）"
}

install_tunnel() {
  [ "$SKIP_TUNNEL" = 1 ] && return 0
  info "Cloudflare Tunnel を登録"
  if [ "$DRY_RUN" = 1 ]; then
    printf '  [dry-run] cloudflared service install <TOKEN>\n'
    [ "$TUNNEL_PROTOCOL" != auto ] && printf '  [dry-run] TUNNEL_TRANSPORT_PROTOCOL=%s の drop-in を作成\n' "$TUNNEL_PROTOCOL" || true
    return 0
  fi

  local unit=/etc/systemd/system/cloudflared.service
  if [ -f "$unit" ] && grep -qF -- "$TUNNEL_TOKEN" "$unit" 2>/dev/null; then
    ok "同一トークンのトンネルは登録済み"
  else
    [ -f "$unit" ] && { cloudflared service uninstall >/dev/null 2>&1 || true; }
    cloudflared service install "$TUNNEL_TOKEN"
    # 生成された unit には token が平文で入る。
    chmod 600 "$unit" 2>/dev/null || true
    ok "トンネルを登録"
  fi

  if [ "$TUNNEL_PROTOCOL" != auto ]; then
    install -d -m 0755 /etc/systemd/system/cloudflared.service.d
    printf '[Service]\nEnvironment=TUNNEL_TRANSPORT_PROTOCOL=%s\n' "$TUNNEL_PROTOCOL" \
      > /etc/systemd/system/cloudflared.service.d/10-protocol.conf
    ok "転送プロトコルを $TUNNEL_PROTOCOL に固定"
  fi
  systemctl daemon-reload
  systemctl enable --now cloudflared

  info "トンネルの接続を待機"
  local deadline=$(( $(date +%s) + 60 ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if journalctl -u cloudflared --since '-2 min' --no-pager 2>/dev/null \
         | grep -q 'Registered tunnel connection'; then
      ok "トンネル接続を確認"; return 0
    fi
    sleep 2
  done
  warn "トンネルの接続を確認できませんでした。UDP/7844 が塞がれている可能性があります。"
  warn "  --tunnel-protocol http2 を付けて再実行してください。"
  warn "  ログ: journalctl -u cloudflared -n 50"
}

# ---------------------------------------------------------------- 仕上げ
write_install_env() {
  [ "$DRY_RUN" = 1 ] && return 0
  install -m 0640 /dev/null /etc/works/install.env
  cat > /etc/works/install.env <<EOF
# NccHub install.sh が書き出した設定。秘密情報は含まない。
WORKS_REPO=$REPO
WORKS_REF=$REF
WORKS_PREFIX=$PREFIX
WORKS_DATA_DIR=$DATA_DIR
WORKS_SVC_USER=$SVC_USER
WORKS_LISTEN=$LISTEN
WORKS_KEEP_RELEASES=$KEEP_RELEASES
WORKS_ADMIN_EMAIL=$ADMIN_EMAIL
EOF
  chmod 0640 /etc/works/install.env
}

save_secrets() {
  [ "$DRY_RUN" = 1 ] && return 0
  [ "${ADMIN_PASSWORD_GENERATED:-0}" = 1 ] || return 0
  umask 077
  cat > "$SECRETS_FILE" <<EOF
NccHub 初期セットアップで生成した秘密情報
生成日時: $(date -u +%Y-%m-%dT%H:%M:%SZ)

管理画面: https://<公開ホスト名>/_/
管理者メール: $ADMIN_EMAIL
管理者パスワード: $ADMIN_PASSWORD

控えたら次のコマンドで確実に削除してください:
  shred -u $SECRETS_FILE
EOF
  chmod 600 "$SECRETS_FILE"
}

print_next_steps() {
  local host='<公開ホスト名>'
  echo
  printf '%s================ インストール完了 ================%s\n' "$C_OK" "$C_0"
  echo
  if [ "${ADMIN_PASSWORD_GENERATED:-0}" = 1 ] && [ "$DRY_RUN" = 0 ]; then
    printf '%s管理者パスワード（この画面にしか出ません）%s\n' "$C_WARN" "$C_0"
    printf '  メール    : %s\n' "$ADMIN_EMAIL"
    printf '  パスワード: %s\n' "$ADMIN_PASSWORD"
    printf '  控えとして %s にも保存しました（控えたら shred -u で消すこと）\n' "$SECRETS_FILE"
    echo
  fi
  cat <<EOF
=== 残っている手作業 ===

[ ] Cloudflare Zero Trust > Networks > Tunnels > 該当トンネル > Public Hostname
      公開ホスト名を設定し、Service を http://$LISTEN にする
[ ] Cloudflare ダッシュボード > SSL/TLS > 暗号化モードを Full にする
[ ] Security > WAF > Rate limiting rules を1本追加
      (http.request.method eq "POST" and http.request.uri.path contains "/api/")
      60 req / 1 min → Block
[ ] Zero Trust > Access > Applications に Self-hosted を2本
      $host/_/*
      $host/api/collections/_superusers/*
      ポリシー: メールが $ADMIN_EMAIL に一致 → Allow（One-time PIN）
      他の公開パスには絶対に Access を掛けないこと（学生には合言葉のみ）
EOF
  [ -z "$EVENT_SLUG" ] && cat <<EOF || true
[ ] 管理画面 https://$host/_/ で events を1件作成
      name / slug / passphrase(8文字以上) / max_video_bytes=2147483648 / submissions_open=true
EOF
  cat <<EOF
[ ] curl -sI https://$host/api/health が 200 を返すことを確認
EOF
  [ "${ADMIN_PASSWORD_GENERATED:-0}" = 1 ] && printf '[ ] %s を控えてから shred -u で削除\n' "$SECRETS_FILE" || true
  echo
  cat <<EOF
=== 運用コマンド ===
  状態確認: systemctl status works-server
  ログ    : journalctl -u works-server -f
  更新    : sudo $SRC_DIR/deploy/update.sh
  バックアップ: sudo $SRC_DIR/deploy/backup.sh
EOF
  [ "$SKIP_TUNNEL" = 1 ] && printf '\n%s注意:%s トンネルは設定していません。外部からは到達できません。\n' "$C_WARN" "$C_0" || true
  [ "$DRY_RUN" = 1 ]     && printf '\n%s注意:%s dry-run のため何も実行していません。\n' "$C_WARN" "$C_0" || true
}

on_error() {
  local rc=$?
  printf '\n%sインストールに失敗しました (exit %d)%s\n' "$C_ERR" "$rc" "$C_0" >&2
  # 切替後に失敗した場合のみ、直前のリリースへ戻す。
  if [ "${ACTIVATED:-0}" = 1 ] && [ -n "${PREV_TARGET:-}" ]; then
    warn "直前のリリースへ戻します: $PREV_TARGET"
    ln -sfn "$PREV_TARGET" "$PREFIX/.current.tmp" 2>/dev/null \
      && mv -Tf "$PREFIX/.current.tmp" "$PREFIX/current" 2>/dev/null \
      && systemctl restart works-server 2>/dev/null || true
  fi
  warn "データ（$PB_DATA）は削除していません。修正して再実行できます。"
  exit "$rc"
}

main() {
  ORIG_ARGS="$*"
  parse_args "$@"
  trap on_error ERR
  trap cleanup_workdir EXIT
  pick_workdir
  resolve_secrets
  preflight
  apt_bootstrap
  ensure_nix
  ensure_user
  sync_source
  build_release
  activate_release
  ensure_superuser
  ensure_backup_credentials
  install_unit
  start_service
  wait_health 120 || die "サービスが応答しません。上のログを確認してください"
  seed_event
  ensure_cloudflared
  install_tunnel
  prune_releases
  write_install_env
  save_secrets
  print_next_steps
}

main "$@"
