#!/usr/bin/env bash
set -euo pipefail
unit=cage-claude-desktop.service
runtime=/run/cage-desktop
slot=${1:-}
case "$slot" in
  "") ;;
  2|3|4) unit="ccbox-claude-desktop@$slot.service"; runtime="/run/ccbox-claude-desktop-$slot" ;;
  *) exit 2 ;;
esac
if systemctl is-active --quiet "$unit"; then
  test -f "$runtime/keyring-ready" || { echo '请先保存并关闭现有 Claude Desktop，再使用 CCBox 账户重新打开。' >&2; exit 1; }
  exit 0
fi
if systemctl is-failed --quiet "$unit"; then
  sudo systemctl reset-failed "$unit"
fi
sudo systemctl start --no-block "$unit"
started=
if [ -n "$slot" ]; then
  trap 'if [ "$started" != 1 ]; then sudo systemctl stop "$unit" >/dev/null 2>&1 || :; fi' EXIT
fi
for i in $(seq 1 150); do
  if [ -p "$runtime/unlock" ]; then break; fi
  if systemctl is-failed --quiet "$unit"; then
    echo '图形会话未能启动。' >&2; exit 1
  fi
  sleep .1
done
test -p "$runtime/unlock" || { echo '等待凭据解锁通道超时。' >&2; exit 1; }
# The internal random key comes exclusively from SSH stdin, never argv or disk.
timeout 10 sh -c 'cat > "$1/unlock"' sh "$runtime"
for i in $(seq 1 300); do
  if systemctl is-active --quiet "$unit"; then started=1; exit 0; fi
  if systemctl is-failed --quiet "$unit"; then
    echo 'Claude Desktop 未能启动或密钥环未能解锁；已有登录数据保持不变。' >&2; exit 1
  fi
  sleep .1
done
echo 'Claude Desktop 启动超时。' >&2
exit 1
