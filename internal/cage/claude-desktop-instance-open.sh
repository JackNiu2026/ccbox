#!/bin/sh
set -eu
test "$#" -eq 1 && test "${#1}" -le 8192
test -n "${CCBOX_DESKTOP_DATA:-}"
case "$1" in
  claude://*)
    exec /usr/bin/claude-desktop "--user-data-dir=$CCBOX_DESKTOP_DATA" "$1"
    ;;
  https://*|http://*|about:blank)
    exec /usr/bin/chromium "--user-data-dir=$HOME/.cache/ccbox-desktop-auth" --incognito --no-first-run --no-default-browser-check --disable-sync -- "$1"
    ;;
  *)
    exec /usr/local/bin/cage-desktop-files --open "$1"
    ;;
esac
