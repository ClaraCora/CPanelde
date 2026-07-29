#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="ClaraCora/CPanelde"
SERVICE_NAME="corade.service"
BINARY_PATH="/usr/local/bin/corade"
CONFIG_DIR="/etc/corade"
CONFIG_FILE="${CONFIG_DIR}/config.yml"
ENV_FILE="${CONFIG_DIR}/agent.env"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}"
ACTION="install"
CONTROL_URL="${CORADE_CONTROL_URL:-}"
COMMUNICATION_KEY="${CORADE_AGENT_TOKEN:-}"
MACHINE_ID=""
KERNEL_TYPE="singbox"
HEALTH_PORT="65530"
BINARY_SOURCE=""
VERSION="latest"

usage() {
  cat <<'HELP'
Corade Agent installer

Usage:
  install.sh [install] --control-url URL --communication-key KEY --machine-id ID [options]
  install.sh upgrade [--version VERSION]

Required for install:
  --control-url URL            CPanel public URL
  --communication-key KEY     Shared Agent communication key
  --machine-id ID             CPanel server ID

Optional:
  --kernel singbox|xray       Default kernel type (default: singbox)
  --health-port PORT          Local health endpoint (default: 65530)
  --binary PATH               Install a local Corade binary
  --version VERSION           GitHub release tag (default: latest)
  --help                      Show this help

CORADE_CONTROL_URL and CORADE_AGENT_TOKEN may be used instead of putting those
values on the command line.
HELP
}

log() { printf '[corade] %s\n' "$*"; }
fail() { printf '[corade] ERROR: %s\n' "$*" >&2; exit 1; }

if [ "${1:-}" = "install" ] || [ "${1:-}" = "upgrade" ]; then
  ACTION="$1"
  shift
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
    --control-url) CONTROL_URL="${2:-}"; shift 2 ;;
    --communication-key) COMMUNICATION_KEY="${2:-}"; shift 2 ;;
    --machine-id) MACHINE_ID="${2:-}"; shift 2 ;;
    --kernel) KERNEL_TYPE="${2:-}"; shift 2 ;;
    --health-port) HEALTH_PORT="${2:-}"; shift 2 ;;
    --binary) BINARY_SOURCE="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[ "$(id -u)" -eq 0 ] || fail "run this installer as root"
command -v systemctl >/dev/null 2>&1 || fail "systemd is required"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

TMP_DIR=$(mktemp -d)
trap 'rm -rf -- "$TMP_DIR"' EXIT
STAGED_BINARY="${TMP_DIR}/corade"
PREVIOUS_BINARY="${TMP_DIR}/corade.previous"
ARTIFACT="corade-linux-${ARCH}"

verify_staged_download() {
  (cd "$TMP_DIR" && sha256sum --check --status "${ARTIFACT}.sha256")
}

try_download() {
  local base_url="$1"
  rm -f "${TMP_DIR}/${ARTIFACT}" "${TMP_DIR}/${ARTIFACT}.sha256"
  if ! curl --fail --location --silent --show-error --retry 3 "${base_url}/${ARTIFACT}" -o "${TMP_DIR}/${ARTIFACT}"; then
    return 1
  fi
  if ! curl --fail --location --silent --show-error --retry 3 "${base_url}/${ARTIFACT}.sha256" -o "${TMP_DIR}/${ARTIFACT}.sha256"; then
    return 1
  fi
  verify_staged_download || return 1
  mv "${TMP_DIR}/${ARTIFACT}" "$STAGED_BINARY"
}

stage_binary() {
  if [ -n "$BINARY_SOURCE" ]; then
    [ -f "$BINARY_SOURCE" ] || fail "binary not found: $BINARY_SOURCE"
    cp "$BINARY_SOURCE" "$STAGED_BINARY"
  else
    if [ -n "$CONTROL_URL" ]; then
      log "downloading the verified Agent binary from CPanel"
      if try_download "${CONTROL_URL%/}/corade-downloads"; then
        chmod 755 "$STAGED_BINARY"
        "$STAGED_BINARY" -v >/dev/null 2>&1 || fail "downloaded binary failed its version check"
        return
      fi
      log "CPanel artifact is unavailable; falling back to GitHub release"
    fi
    log "downloading ${REPOSITORY} release ${VERSION}"
    try_download "https://github.com/${REPOSITORY}/releases/download/${VERSION}" || fail "could not download a verified Agent binary"
  fi
  chmod 755 "$STAGED_BINARY"
  "$STAGED_BINARY" -v >/dev/null 2>&1 || fail "downloaded binary failed its version check"
}

service_is_healthy() {
  systemctl is-active --quiet "$SERVICE_NAME" || return 1
  if [ "$HEALTH_PORT" != "0" ]; then
    curl --fail --silent "http://127.0.0.1:${HEALTH_PORT}/healthz" >/dev/null 2>&1 || return 1
  fi
}

wait_until_healthy() {
  for _ in $(seq 1 30); do
    if service_is_healthy; then
      return 0
    fi
    sleep 1
  done
  return 1
}

install_binary_and_restart() {
  if [ -f "$BINARY_PATH" ]; then
    cp "$BINARY_PATH" "$PREVIOUS_BINARY"
  fi
  systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
  install -m 755 "$STAGED_BINARY" "$BINARY_PATH"
  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME" >/dev/null
  systemctl restart "$SERVICE_NAME"
  if wait_until_healthy; then
    return
  fi
  journalctl -u "$SERVICE_NAME" -n 50 --no-pager >&2 || true
  if [ -f "$PREVIOUS_BINARY" ]; then
    log "new Agent did not become healthy; restoring the previous binary"
    install -m 755 "$PREVIOUS_BINARY" "$BINARY_PATH"
    systemctl restart "$SERVICE_NAME" || true
  fi
  fail "Agent did not become healthy"
}

if [ "$ACTION" = "upgrade" ]; then
  [ -f "$CONFIG_FILE" ] && [ -f "$ENV_FILE" ] && [ -f "$SERVICE_FILE" ] || fail "Corade is not installed; run install first"
  HEALTH_PORT=$(awk '/^[[:space:]]*health_port:/ {print $2; exit}' "$CONFIG_FILE")
  HEALTH_PORT=${HEALTH_PORT:-65530}
  stage_binary
  install_binary_and_restart
  log "upgrade completed"
  log "service: ${SERVICE_NAME}"
  exit 0
fi

[ -n "$CONTROL_URL" ] || fail "--control-url or CORADE_CONTROL_URL is required"
[ -n "$MACHINE_ID" ] || fail "--machine-id is required"
[ "${#COMMUNICATION_KEY}" -ge 32 ] || fail "--communication-key or CORADE_AGENT_TOKEN must contain at least 32 characters"
[[ "$CONTROL_URL" =~ ^https?://[^[:space:]]+$ ]] || fail "control URL must start with http:// or https:// and contain no whitespace"
[[ "$MACHINE_ID" =~ ^[A-Za-z0-9_-]+$ ]] || fail "machine ID contains unsupported characters"
[[ "$COMMUNICATION_KEY" =~ ^[A-Za-z0-9_+/=-]+$ ]] || fail "communication key contains unsupported characters"
[[ "$HEALTH_PORT" =~ ^[0-9]+$ ]] || fail "health port must be a number"
case "$KERNEL_TYPE" in singbox|xray) ;; *) fail "kernel must be singbox or xray" ;; esac

stage_binary
mkdir -p "$CONFIG_DIR"
if [ -f "$CONFIG_FILE" ]; then
  cp "$CONFIG_FILE" "${CONFIG_FILE}.backup.$(date +%Y%m%d%H%M%S)"
fi

cat >"${TMP_DIR}/config.yml" <<EOF
control:
  mode: "device-platform"
  url: "${CONTROL_URL%/}"
  token_env: "CORADE_AGENT_TOKEN"
  machine_id: "${MACHINE_ID}"

kernel:
  type: "${KERNEL_TYPE}"
  log_level: "warn"

log:
  level: "info"
  output: "stdout"

health_port: ${HEALTH_PORT}
EOF

printf 'CORADE_AGENT_TOKEN=%s\n' "$COMMUNICATION_KEY" >"${TMP_DIR}/agent.env"
chmod 600 "${TMP_DIR}/agent.env"

cat >"${TMP_DIR}/${SERVICE_NAME}" <<EOF
[Unit]
Description=Corade device platform Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${CONFIG_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${BINARY_PATH} -c ${CONFIG_FILE}
Restart=always
RestartSec=5
LimitNOFILE=1048576
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF

install -m 600 "${TMP_DIR}/config.yml" "$CONFIG_FILE"
install -m 600 "${TMP_DIR}/agent.env" "$ENV_FILE"
install -m 644 "${TMP_DIR}/${SERVICE_NAME}" "$SERVICE_FILE"
install_binary_and_restart

log "installation completed"
log "service: ${SERVICE_NAME}"
log "config: ${CONFIG_FILE}"
log "logs: journalctl -u ${SERVICE_NAME} -f"
