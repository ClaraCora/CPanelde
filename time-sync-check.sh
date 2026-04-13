#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[%s] %s\n' "$1" "$2"
}

info() {
  log INFO "$1"
}

warn() {
  log WARN "$1"
}

error() {
  log ERROR "$1" >&2
}

require_root() {
  if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    error "请使用 root 运行此脚本"
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

install_chrony() {
  local pm
  pm="$(detect_pkg_manager)"

  case "$pm" in
    apt)
      info "使用 apt 安装 chrony"
      apt-get update
      DEBIAN_FRONTEND=noninteractive apt-get install -y chrony
      ;;
    dnf)
      info "使用 dnf 安装 chrony"
      dnf install -y chrony
      ;;
    yum)
      info "使用 yum 安装 chrony"
      yum install -y chrony
      ;;
    apk)
      info "使用 apk 安装 chrony"
      apk add --no-cache chrony
      ;;
    *)
      error "未识别的包管理器，无法自动安装 chrony"
      exit 1
      ;;
  esac
}

service_exists() {
  local name="$1"
  systemctl list-unit-files "$name" 2>/dev/null | grep -q "^${name}"
}

enable_chrony_service() {
  if service_exists chrony.service; then
    info "启用并启动 chrony.service"
    systemctl enable --now chrony.service
    return
  fi

  if service_exists chronyd.service; then
    info "启用并启动 chronyd.service"
    systemctl enable --now chronyd.service
    return
  fi

  error "已安装 chrony，但未找到 [chrony.service](time-sync-check.sh:1) 或 [chronyd.service](time-sync-check.sh:1)"
  exit 1
}

ensure_chrony() {
  if command -v chronyc >/dev/null 2>&1; then
    info "检测到 chrony 已安装"
  else
    warn "未检测到 chrony，开始安装"
    install_chrony
  fi

  enable_chrony_service
}

force_sync() {
  if command -v chronyc >/dev/null 2>&1; then
    info "执行 chrony 立即校时"
    chronyc makestep || warn "chronyc makestep 执行失败，请稍后手动重试"
  fi
}

show_status() {
  echo
  info "当前 timedatectl 状态"
  if command -v timedatectl >/dev/null 2>&1; then
    timedatectl status || true
  else
    warn "系统未提供 timedatectl"
  fi

  echo
  info "当前 chrony tracking 状态"
  if command -v chronyc >/dev/null 2>&1; then
    chronyc tracking || true
    echo
    info "当前 chrony sources 状态"
    chronyc sources -v || true
  else
    warn "系统未提供 chronyc"
  fi
}

main() {
  require_root
  info "开始检查服务器时间同步状态"
  show_status
  echo
  ensure_chrony
  force_sync
  sleep 2
  echo
  info "校时完成，重新输出状态"
  show_status
  echo
  info "如果 [System clock synchronized](time-sync-check.sh:1) 仍短暂显示 no，但 [chronyc tracking](time-sync-check.sh:1) 中偏移接近 0，通常说明系统已经基本同步"
}

main "$@"
