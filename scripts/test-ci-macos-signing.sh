#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# Exercise the runner checks without signing credentials or touching keychains.
fixture=$(mktemp -d)
trap 'rmdir "$fixture"' EXIT
unset CAGE_MACOS_SIGNING_STATE CAGE_MACOS_P12_BASE64
export GITHUB_ACTIONS=true RUNNER_TEMP="$fixture"
uname() {
    case "$1" in
        -s) printf '%s\n' "$TEST_SYSTEM" ;;
        -m) printf '%s\n' "$TEST_MACHINE" ;;
        *) return 1 ;;
    esac
}
export -f uname

for pair in arm64:arm64 amd64:x86_64; do
    export CCBOX_MACOS_ARCH="${pair%%:*}" TEST_MACHINE="${pair#*:}" TEST_SYSTEM=Darwin
    bash scripts/ci-macos-signing.sh cleanup
    if diagnostic=$(bash scripts/ci-macos-signing.sh run 2>&1); then exit 1; fi
    [[ "$diagnostic" == *'Missing cage-release secret: CAGE_MACOS_P12_BASE64'* ]]
done

if TEST_MACHINE=arm64 CCBOX_MACOS_ARCH=amd64 bash scripts/ci-macos-signing.sh cleanup; then exit 1; fi
if TEST_SYSTEM=Linux bash scripts/ci-macos-signing.sh cleanup; then exit 1; fi
