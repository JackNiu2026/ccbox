#!/usr/bin/env bash
# Validate the actual downloaded distribution without installing or launching it.
set -euo pipefail
directory="${1:?Usage: scripts/verify-macos-release.sh <release-directory> [--unsigned-preview]}"
# An unsigned preview has no Developer ID, notarization or Gatekeeper approval;
# everything else (checksums, version and corresponding source) is verified.
preview=0
case "${2:-}" in '') ;; --unsigned-preview) preview=1 ;; *) echo 'Unknown option.' >&2; exit 2 ;; esac
# stapler contacts Apple's CloudKit even for a stapled artifact. Hosted runners
# can lose this connection; retry service failures without bypassing validation.
validate_ticket() {
  local attempt status
  for attempt in 1 2 3; do
    if xcrun stapler validate "$1"; then return 0; else status=$?; fi
    if [[ "$status" != 68 || "$attempt" == 3 ]]; then return "$status"; fi
    echo "Apple ticket service unavailable; retry $attempt/2." >&2
    sleep 5
  done
}
shopt -s nullglob
arm=("$directory"/CCBox-*-macos-arm64.dmg)
intel=("$directory"/CCBox-*-macos-amd64.dmg)
if [[ ${#arm[@]} -gt 1 || ${#intel[@]} -gt 1 || $(( ${#arm[@]} + ${#intel[@]} )) == 0 ]]; then
  echo 'Expected at most one CCBox DMG per macOS architecture.' >&2
  exit 1
fi
# The fallback expansion supports empty arrays in the macOS system Bash.
archives=(${arm[@]+"${arm[@]}"} ${intel[@]+"${intel[@]}"})
sources=("$directory"/mihomo-v*-source.tar.gz)
if [[ ${#sources[@]} != 1 ]]; then echo 'Expected exactly one Mihomo source archive.' >&2; exit 1; fi
checksum_input="$(mktemp)"
mount=''
cleanup() {
  rm -f "$checksum_input"
  if [[ -n "$mount" ]]; then
    hdiutil detach "$mount" >/dev/null 2>&1 || true
    rmdir "$mount" 2>/dev/null || true
  fi
}
trap cleanup EXIT
script_dir="$(cd "$(dirname "$0")" && pwd)"
verify_archive() {
  local archive="$1" arch version machine bundle guest_source embedded_guest executable status
  version="${archive##*/CCBox-}"
  arch="${version##*-macos-}"
  arch="${arch%.dmg}"
  version="${version%-macos-"$arch".dmg}"
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || return 1
  embedded_guest="cage-macos-guest-$version-source.tar.gz"
  case "$arch" in
    arm64) machine=arm64; guest_source="$directory/$embedded_guest" ;;
    amd64) machine=x86_64; guest_source="$directory/cage-macos-guest-$version-amd64-source.tar.gz" ;;
    *) return 1 ;;
  esac
  [[ -f "$guest_source" ]] || { echo 'Expected the matching macOS guest corresponding source archive.' >&2; return 1; }
  awk -v archive="${archive##*/}" -v source="${sources[0]##*/}" -v guest="${guest_source##*/}" '
    NF == 2 && ($2 == archive || $2 == "*" archive) { archives++; print }
    NF == 2 && ($2 == source || $2 == "*" source) { sources++; print }
    NF == 2 && ($2 == guest || $2 == "*" guest) { guests++; print }
    END { if (archives != 1 || sources != 1 || guests != 1) exit 1 }
  ' "$directory/SHA256SUMS" > "$checksum_input"
  (cd "$directory" && shasum -a 256 --check "$checksum_input")
  if [[ $preview == 0 ]]; then
    codesign --verify --strict --verbose=2 "$archive"
    validate_ticket "$archive"
    spctl --assess --type open --context context:primary-signature --verbose=2 "$archive"
  fi
  mount="$(mktemp -d)"
  hdiutil attach -readonly -nobrowse -mountpoint "$mount" "$archive" >/dev/null
  bundle="$mount/ccBox.app"
  [[ "$(/usr/libexec/PlistBuddy -c 'Print :CageVersion' "$bundle/Contents/Info.plist")" == "$version" ]]
  [[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$bundle/Contents/Info.plist")" == org.ccbox.desktop ]]
  if [[ $preview == 0 ]]; then
    codesign --verify --deep --strict --verbose=2 \
      -R='anchor apple generic and identifier "org.ccbox.desktop" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists' "$bundle"
    validate_ticket "$bundle"
    spctl --assess --type execute --verbose=2 "$bundle"
  else
    codesign --verify --deep --strict --verbose=2 "$bundle"
  fi
  for executable in Contents/MacOS/Cage Contents/Resources/cage Contents/Resources/mihomo Contents/Resources/CageUpdateInstaller; do
    [[ "$(lipo -archs "$bundle/$executable")" == "$machine" ]] || { echo "Architecture mismatch in $executable" >&2; return 1; }
  done
  if [[ "$arch" == amd64 ]]; then
    for executable in qemu-system-x86_64 qemu-img; do
      [[ "$(lipo -archs "$bundle/Contents/Resources/intel-macos/runtime/bin/$executable")" == x86_64 ]]
    done
  fi
  cmp "$guest_source" "$bundle/Contents/Resources/guest-os/sources/$embedded_guest"
  node "$script_dir/guest-os/collect-macos-sshfs-sources.mjs" verify "$guest_source" \
    "$bundle/Contents/Resources/guest-os/project/cage-sshfs" "$version"
  hdiutil detach "$mount" >/dev/null
  rmdir "$mount"
  mount=''
  status=notarized
  if [[ $preview == 1 ]]; then status='unsigned preview'; fi
  echo "Verified $status checksums, signature, architecture and corresponding source: CCBox $version ($arch)"
}
for archive in "${archives[@]}"; do verify_archive "$archive"; done
