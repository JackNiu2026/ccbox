#!/usr/bin/env bash
# Build, notarize, and package a Developer ID distribution. No installation.
set -euo pipefail
cd "$(dirname "$0")/.."
node scripts/check-ccbox-brand.mjs
machine="$(uname -m)"
case "$(uname -s)-$machine" in
  Darwin-arm64) arch=arm64; chip='Apple Silicon'; chip_zh='Apple Silicon Mac' ;;
  Darwin-x86_64)
    [[ "$(/usr/sbin/sysctl -in sysctl.proc_translated)" != 1 ]] || { echo 'Build Intel packages on a native Intel Mac.' >&2; exit 1; }
    arch=amd64; chip='an Intel Mac with VT-x'; chip_zh='Intel Mac（需支持 VT-x 硬件虚拟化）' ;;
  *) echo 'The supported release targets are Apple Silicon and Intel macOS 14+.' >&2; exit 1 ;;
esac
case "${1:-}" in ''|--from-notarized-app|--unsigned-preview) ;; *) echo 'Usage: scripts/package-macos.sh [--from-notarized-app | --unsigned-preview]' >&2; exit 2 ;; esac
# An unsigned preview is ad-hoc signed and not notarized. Gatekeeper asks the
# user to allow it in System Settings, and in-app updates stay unavailable.
preview=0
if [[ "${1:-}" == --unsigned-preview ]]; then
  preview=1
  export CAGE_SIGNING_IDENTITY=-
  unset CAGE_PROVISIONING_PROFILE
else
  : "${CAGE_SIGNING_IDENTITY:?Set CAGE_SIGNING_IDENTITY to your Developer ID Application identity.}"
  : "${CAGE_NOTARY_PROFILE:?Set CAGE_NOTARY_PROFILE to a notarytool keychain profile.}"
  notary_credentials=(--keychain-profile "$CAGE_NOTARY_PROFILE")
  if [[ -n "${CAGE_NOTARY_KEYCHAIN:-}" ]]; then
    notary_credentials+=(--keychain "$CAGE_NOTARY_KEYCHAIN")
  fi
  if [[ "$CAGE_SIGNING_IDENTITY" == - ]]; then echo 'Release packages require a Developer ID identity.' >&2; exit 1; fi
fi
version="$(tr -d '\n\r' < VERSION)"
release="$PWD/_output/releases/$version"
bundle="$PWD/_output/ccBox.app"
mkdir -p "$release"
if [[ $preview == 1 ]]; then
  bash scripts/build-app.sh
elif [[ "${1:-}" != --from-notarized-app ]]; then
  bash scripts/build-app.sh
  ditto -c -k --keepParent "$bundle" "$release/CCBox-notarization.zip"
  xcrun notarytool submit "$release/CCBox-notarization.zip" \
    "${notary_credentials[@]}" --wait --timeout 20m \
    --output-format json > "$release/app-notarization.json"
  node -e 'const r=JSON.parse(require("fs").readFileSync(process.argv[1])); if(r.status!=="Accepted") throw new Error(`Apple notarization: ${r.status}; submission ${r.id}`)' "$release/app-notarization.json"
  xcrun stapler staple "$bundle"
fi
actual_version="$(/usr/libexec/PlistBuddy -c 'Print :CageVersion' "$bundle/Contents/Info.plist")"
if [[ "$actual_version" != "$version" ]]; then echo 'App version differs from VERSION; rebuild before packaging.' >&2; exit 1; fi
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleDisplayName' "$bundle/Contents/Info.plist")" == CCBox ]]
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$bundle/Contents/Info.plist")" == org.ccbox.desktop ]]
for executable in Contents/MacOS/Cage Contents/Resources/cage Contents/Resources/mihomo Contents/Resources/CageUpdateInstaller; do
  [[ "$(lipo -archs "$bundle/$executable")" == "$machine" ]] || { echo "Architecture mismatch in $executable." >&2; exit 1; }
done
if [[ "$arch" == amd64 ]]; then
  for executable in qemu-system-x86_64 qemu-img; do
    [[ "$(lipo -archs "$bundle/Contents/Resources/intel-macos/runtime/bin/$executable")" == x86_64 ]]
  done
fi
codesign --verify --deep --strict --verbose=2 "$bundle"
if [[ $preview == 0 ]]; then
  xcrun stapler validate "$bundle"
  spctl --assess --type execute --verbose=2 "$bundle"
fi
"$bundle/Contents/Resources/cage" verify-web-assets
guest_source="cage-macos-guest-$version-source.tar.gz"
guest_materials="$bundle/Contents/Resources/guest-os/sources/$guest_source"
if [[ "$arch" == amd64 ]]; then guest_source="cage-macos-guest-$version-amd64-source.tar.gz"; fi
node scripts/guest-os/collect-macos-sshfs-sources.mjs verify "$guest_materials" \
  "$bundle/Contents/Resources/guest-os/project/cage-sshfs" "$version"
# Copy only the source frozen by this App build, including the final signed
# helper hash. Packaging must not substitute newer Homebrew source material.
cp "$guest_materials" "$release/$guest_source"

stage="$(mktemp -d "$PWD/_output/dmg-stage.XXXXXX")"
trap 'rm -rf "$stage"' EXIT
ditto "$bundle" "$stage/ccBox.app"
ln -s /Applications "$stage/Applications"
cat > "$stage/安装说明 - Read Me.txt" <<EOF
CCBox — 本机隔离编码工作区

1. 将 ccBox.app 拖入 Applications（应用程序）。
2. 从应用程序打开 CCBox，设置本地账户，并按首次引导选择客机系统、存储位置和代理。
3. 添加自己的 Clash/Mihomo YAML 订阅或配置并选择节点，创建和启动环境。首次需下载系统镜像和工具，请保持网络连接。
4. 打开项目或新建终端，输入 claude，并在 CCBox 浏览器内登录自己的 Claude 账户。

需要 $chip_zh、macOS 14 或更新版本。无需安装 Go、Node.js 或 Clash。
代理服务与 Claude 账户需自行准备。具体版本与发布状态以发行说明为准。
共享项目中的文件修改会直接写回 Mac；请备份重要项目。
已有环境：保存工作，在 CCBox 中停止环境并退出。将新版应用拖到原安装位置并选择「替换」。
用原 macOS 帐户启动并沿用原配置与数据盘。若意外出现首次设置，请先核对原存储位置。

Drag ccBox.app into Applications, then open it from Applications.
Complete the local account and environment setup, add your own Clash/Mihomo
YAML subscription or configuration, select a node, and start the environment.
Initial setup downloads the selected guest system and developer tools.
Requires $chip and macOS 14+. Your own proxy and Claude account are required.
To upgrade, save your work, stop the environment, and quit CCBox. Replace the app
in its original location and reopen it with the same macOS account.

安装及故障排查见 CCBox 源码中的 docs/getting-started.md。
EOF
artifact="CCBox-$version-macos-$arch.dmg"
hdiutil create -volname CCBox -srcfolder "$stage" -ov -format UDZO "$release/$artifact"
if [[ $preview == 0 ]]; then
  codesign --force --sign "$CAGE_SIGNING_IDENTITY" --timestamp "$release/$artifact"
  xcrun notarytool submit "$release/$artifact" "${notary_credentials[@]}" \
    --wait --timeout 20m --output-format json > "$release/dmg-notarization.json"
  node -e 'const r=JSON.parse(require("fs").readFileSync(process.argv[1])); if(r.status!=="Accepted") throw new Error(`Apple notarization: ${r.status}; submission ${r.id}`)' "$release/dmg-notarization.json"
  xcrun stapler staple "$release/$artifact"
  xcrun stapler validate "$release/$artifact"
  spctl --assess --type open --context context:primary-signature --verbose=2 "$release/$artifact"
fi

# Distribute the corresponding unmodified Mihomo source beside its executable.
core_version="$(sed -n 's/^version=//p' scripts/fetch-proxy-core.sh)"
core_source="mihomo-$core_version-source.tar.gz"
curl --fail --location --retry 2 --connect-timeout 15 --max-time 180 \
  "https://github.com/MetaCubeX/mihomo/archive/refs/tags/$core_version.tar.gz" \
  -o "$release/$core_source.part"
mv "$release/$core_source.part" "$release/$core_source"
tar -tzf "$release/$core_source" > /dev/null
(cd "$release" && shasum -a 256 "$artifact" "$core_source" "$guest_source" > SHA256SUMS)
node scripts/cleanup-apps.mjs --keep "$bundle" --root "$PWD/_output"
printf 'Verified release artifacts:\n%s/%s\n%s/%s\n%s/%s\n%s/SHA256SUMS\n' "$release" "$artifact" "$release" "$core_source" "$release" "$guest_source" "$release"
