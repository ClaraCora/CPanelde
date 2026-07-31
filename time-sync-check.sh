#!/bin/sh
set -eu

log() { printf '[%s] %s\n' "$1" "$2"; }
info() { log INFO "$1"; }
warn() { log WARN "$1"; }
error() { log ERROR "$1" >&2; }

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    error "Run this script as root"
    exit 1
  fi
}

detect_pkg_manager() {
  if command -v apt-get >/dev/null 2>&1; then
    echo apt
  elif command -v dnf >/dev/null 2>&1; then
    echo dnf
  elif command -v yum >/dev/null 2>&1; then
    echo yum
  elif command -v apk >/dev/null 2>&1; then
    echo apk
  else
    echo unknown
  fi
}

detect_init_system() {
  if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    echo systemd
  elif command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
    echo openrc
  else
    echo unknown
  fi
}

install_chrony() {
  case "$(detect_pkg_manager)" in
    apt)
      info "Installing chrony with apt"
      apt-get update
      DEBIAN_FRONTEND=noninteractive apt-get install -y chrony
      ;;
    dnf)
      info "Installing chrony with dnf"
      dnf install -y chrony
      ;;
    yum)
      info "Installing chrony with yum"
      yum install -y chrony
      ;;
    apk)
      info "Installing chrony with apk"
      apk add --no-cache chrony
      ;;
    *)
      error "No supported package manager was found"
      exit 1
      ;;
  esac
}

enable_chrony_service() {
  init_system=$(detect_init_system)
  case "$init_system" in
    systemd)
      if systemctl list-unit-files chrony.service 2>/dev/null | grep -q '^chrony.service'; then
        systemctl enable --now chrony.service
      elif systemctl list-unit-files chronyd.service 2>/dev/null | grep -q '^chronyd.service'; then
        systemctl enable --now chronyd.service
      else
        error "chrony is installed but no systemd service was found"
        exit 1
      fi
      ;;
    openrc)
      if [ -x /etc/init.d/chronyd ]; then
        chrony_service=chronyd
      elif [ -x /etc/init.d/chrony ]; then
        chrony_service=chrony
      else
        error "chrony is installed but no OpenRC service was found"
        exit 1
      fi
      rc-update add "$chrony_service" default >/dev/null
      rc-service "$chrony_service" start
      ;;
    *)
      error "No supported service manager was found"
      exit 1
      ;;
  esac
}

ensure_chrony() {
  if command -v chronyc >/dev/null 2>&1; then
    info "chrony is already installed"
  else
    warn "chrony is not installed"
    install_chrony
  fi
  enable_chrony_service
}

force_sync() {
  info "Requesting an immediate clock step"
  chronyc makestep || warn "chronyc makestep failed; retry after the daemon has synchronized"
}

show_status() {
  if command -v timedatectl >/dev/null 2>&1; then
    timedatectl status || true
  fi
  if command -v chronyc >/dev/null 2>&1; then
    chronyc tracking || true
    chronyc sources -v || true
  fi
}

main() {
  require_root
  info "Checking server time synchronization"
  show_status
  ensure_chrony
  force_sync
  sleep 2
  show_status
}

main "$@"
