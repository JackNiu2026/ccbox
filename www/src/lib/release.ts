import releaseData from '../data/release.json';
import type { CageLocale } from '../i18n/languages';

type PlatformRelease = { version: string; releaseURL?: string; downloadURL?: string };
export type ReleaseMetadata = PlatformRelease & {
  windowsDownloadURL?: string; linuxDownloadURL?: string; androidDownloadURL?: string;
  macosPreview?: PlatformRelease; windowsPreview?: PlatformRelease; linuxPreview?: PlatformRelease; androidPreview?: PlatformRelease;
};
export type Platform = {
  name: string; icon: string; requirements: string; guideURL: string;
  format?: string; url?: string; version?: string; notes?: string; preview?: string; localBuild?: boolean;
};

const staged = import.meta.glob<ReleaseMetadata>('../data/local-release.json', { eager: true, import: 'default' });
export const published: ReleaseMetadata = staged['../data/local-release.json'] ?? releaseData;

const previews = [published.macosPreview, published.windowsPreview, published.linuxPreview, published.androidPreview];
for (const url of [published.releaseURL, published.downloadURL, published.windowsDownloadURL, published.linuxDownloadURL, published.androidDownloadURL, ...previews.flatMap(preview => [preview?.downloadURL, preview?.releaseURL])]) {
  if (url && (!/^\/downloads\/[\w./-]+$/.test(url) || url.split('/').some(part => part === '.' || part === '..'))) {
    throw new Error('CCBox release links must point to local downloads.');
  }
}

function previewLabels(locale: CageLocale) {
  if (locale === 'zh') return ['预览版 · 未经 Apple 公证', '预览版', '预览版 · 调试签名'];
  if (locale === 'zh-TW') return ['預覽版 · 未經 Apple 公證', '預覽版', '預覽版 · 偵錯簽章'];
  return ['Preview · not Apple-notarized', 'Preview', 'Preview · debug-signed'];
}

function platformRelease(download: string | undefined, preview: PlatformRelease | undefined, label: string) {
  if (download) return { url: download, version: published.version, notes: published.releaseURL };
  return { url: preview?.downloadURL, version: preview?.version, notes: preview?.releaseURL, preview: preview?.downloadURL ? label : undefined };
}

export function platformsFor(locale: CageLocale, unsignedLabel: string, base: string): Platform[] {
  const [notarized, preview, debug] = previewLabels(locale);
  const install = `${base}docs/installation/`;
  const mobile = `${base}docs/mobile/`;
  return [
    { name: 'macOS', icon: 'apple', requirements: 'Apple Silicon · macOS 14+', format: '.dmg', ...platformRelease(published.downloadURL, published.macosPreview, notarized), guideURL: install },
    { name: 'Windows', icon: 'windows', requirements: 'Windows 10 22H2 / 11 · x64', format: '.exe', ...platformRelease(published.windowsDownloadURL, published.windowsPreview, unsignedLabel), guideURL: install },
    { name: 'Linux', icon: 'linux', requirements: 'Debian 12 / Ubuntu 22.04 / 24.04 · x86_64', format: '.deb', ...platformRelease(published.linuxDownloadURL, published.linuxPreview, preview), guideURL: install },
    { name: 'Android', icon: 'android', requirements: 'Android 8.0+ · arm64', format: '.apk', ...platformRelease(published.androidDownloadURL, published.androidPreview, debug), guideURL: mobile },
    { name: 'iOS', icon: 'apple', requirements: 'iOS 17+ · Secure Enclave', localBuild: true, guideURL: mobile },
  ];
}
