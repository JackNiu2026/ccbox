#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || { echo 'Package on x86_64 Linux.' >&2; exit 1; }
case "${1:-}" in '') bash scripts/build-linux.sh ;; --skip-build) ;; *) echo 'Usage: scripts/package-linux.sh [--skip-build]' >&2; exit 2 ;; esac
version=$(tr -d '\r\n' < VERSION)
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid VERSION.' >&2; exit 1; }
[[ "$(cat _output/linux-amd64/app/VERSION)" == "$version" ]] || { echo 'Build version does not match VERSION.' >&2; exit 1; }
deb_version="${version/-/\~}"
output="$PWD/_output/releases/$version/linux"
mkdir -p "$output"
stage=$(mktemp -d "$PWD/_output/linux-amd64/package.XXXXXX")
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage"/{DEBIAN,usr/bin,usr/lib/ccbox,usr/share/applications,usr/share/icons/hicolor/512x512/apps,usr/share/doc/ccbox}
cp -a _output/linux-amd64/app/. "$stage/usr/lib/ccbox/"
# The distro already owns /usr/bin/cage (a Wayland compositor).
# Keep the backend adjacent to Mihomo, with a distinct public command.
ln -s ../lib/ccbox/cage "$stage/usr/bin/ccbox"
ln -s ../lib/ccbox/cage-desktop "$stage/usr/bin/ccbox-desktop"
install -m 644 packaging/linux/org.ccbox.desktop.desktop "$stage/usr/share/applications/"
install -m 644 logo.png "$stage/usr/share/icons/hicolor/512x512/apps/org.ccbox.desktop.png"
install -m 644 docs/linux.md "$stage/usr/share/doc/ccbox/README.md"
install -m 755 packaging/linux/preinst "$stage/DEBIAN/preinst"
install -m 755 packaging/linux/preinst "$stage/DEBIAN/prerm"
# dpkg-shlibdeps derives glibc/GTK dependencies from the build distribution.
# Use Ubuntu 22.04 for releases; a newer build must not claim an older ABI.
mkdir -p "$stage/debian"
# Set the distributor's contact for the package.
maintainer="${CCBOX_DEB_MAINTAINER:-CCBox <build@localhost>}"
if [[ "$maintainer" == *$'\r'* || "$maintainer" == *$'\n'* ]]; then
  echo 'CCBOX_DEB_MAINTAINER must be a single line.' >&2
  exit 1
fi
printf 'Source: ccbox\nSection: devel\nPriority: optional\nMaintainer: %s\n\nPackage: ccbox\nArchitecture: amd64\nDescription: CCBox desktop\n' "$maintainer" > "$stage/debian/control"
dependencies=$(cd "$stage" && dpkg-shlibdeps -O usr/lib/ccbox/cage-desktop | sed -n 's/^shlibs:Depends=//p')
[[ -n "$dependencies" ]] || { echo 'Cannot determine native dependencies.' >&2; exit 1; }
rm -rf "$stage/debian"
cat > "$stage/DEBIAN/control" <<EOF
Package: ccbox
Version: $deb_version
Section: devel
Priority: optional
Architecture: amd64
Maintainer: $maintainer
Homepage: https://ccbox.org/
Depends: $dependencies, libwebkit2gtk-4.1-0 (>= 2.40), libglib2.0-bin, ca-certificates, systemd
Suggests: qemu-system-x86, qemu-utils, ovmf, openssh-client, iproute2, git
Recommends: dbus-user-session, fonts-noto-cjk, libnotify-bin, zenity | kdialog
Description: CCBox isolated coding workspace with a persistent Linux VM
 CCBox combines projects, terminals, a browser and an editor in a native
 desktop window. Select a local virtual machine or connect to a CCBox server over SSH.
EOF
deb="$output/CCBox-$version-linux-amd64.deb"
dpkg-deb --root-owner-group --build "$stage" "$deb"
cp _output/linux-deps/mihomo-v1.19.33-source.tar.gz "$output/"
(cd "$output" && sha256sum "$(basename "$deb")" mihomo-v1.19.33-source.tar.gz > SHA256SUMS)
printf 'Packaged: %s\n' "$deb"
