import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { lstat, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { readSiteRelease, stageDownloads } from './stage-downloads.mjs';

const version = (await readFile(new URL('../../VERSION', import.meta.url), 'utf8')).trim();
const dependencies = JSON.parse(await readFile(new URL('../../packaging/linux/dependencies.json', import.meta.url), 'utf8'));
const installer = `CCBox-${version}-linux-amd64.deb`;
const source = dependencies.mihomoSource.file;
const digest = data => createHash('sha256').update(data).digest('hex');

function updateAsset(name, data, manual = false) {
  return {
    name, size: Buffer.byteLength(data), browser_download_url: `https://ccbox.org/downloads/${version}/${name}`,
    state: 'uploaded', ...(manual ? { manual: true } : {}),
  };
}

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'ccbox-download-test-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const input = join(root, 'platform');
  const site = join(root, 'site');
  const notes = join(root, 'notes.md');
  await mkdir(input);
  await writeFile(notes, '# CCBox\n\nLinux release fixture.\n');
  const assets = { [installer]: 'installer fixture', [source]: 'source fixture' };
  for (const [name, data] of Object.entries(assets)) await writeFile(join(input, name), data);
  await writeFile(join(input, 'SHA256SUMS'), Object.entries(assets).map(([name, data]) => `${digest(data)}  ${name}\n`).join(''));
  return { input, site, notes, assets };
}

test('stages one platform with same-origin links, notes and verified source materials', async t => {
  const f = await fixture(t);
  assert.deepEqual(await readSiteRelease(f.site, { version }), { version });
  const result = await stageDownloads({ inputs: [f.input], siteDirectory: f.site, notes: f.notes });
  const base = `/downloads/${version}/`;
  const checksums = await readFile(join(result.directory, 'SHA256SUMS'), 'utf8');
  assert.deepEqual(await readSiteRelease(f.site, { version }), {
    version, linuxDownloadURL: base + installer, releaseURL: base + 'release-notes.txt',
    updateAssets: [updateAsset(installer, f.assets[installer]), updateAsset('SHA256SUMS', checksums)],
  });
  assert.equal(await readFile(join(result.directory, installer), 'utf8'), f.assets[installer]);
  assert.equal(await readFile(join(result.directory, source), 'utf8'), f.assets[source]);
  assert.match(checksums, new RegExp(digest(f.assets[installer])));
  assert.ok((await readFile(join(result.directory, 'release-notes.txt'), 'utf8')).includes(checksums));
  await assert.rejects(stageDownloads({ inputs: [f.input], siteDirectory: f.site, notes: f.notes }), /existing release assets are never overwritten/);
});

test('does not expose a corrupted package or create active metadata', async t => {
  const f = await fixture(t);
  await writeFile(join(f.input, installer), 'corrupt');
  await assert.rejects(stageDownloads({ inputs: [f.input], siteDirectory: f.site, notes: f.notes }), /SHA-256 mismatch/);
  assert.deepEqual(await readSiteRelease(f.site, { version }), { version });
  await assert.rejects(lstat(join(f.site, 'public', 'downloads', version)), { code: 'ENOENT' });
});

test('rejects release notes that direct users to the upstream project', async t => {
  const f = await fixture(t);
  await writeFile(f.notes, 'Download at https://github.com/Lee-xeo/Claude-Cage/releases\n');
  await assert.rejects(stageDownloads({ inputs: [f.input], siteDirectory: f.site, notes: f.notes }), /upstream URL/);
  assert.deepEqual(await readSiteRelease(f.site, { version }), { version });
  await assert.rejects(lstat(join(f.site, 'public', 'downloads', version)), { code: 'ENOENT' });
});

test('stages the current Android release APK as a same-origin download', async t => {
  const f = await fixture(t);
  const apk = `CCBox-${version}-android-arm64.apk`;
  const contents = 'verified release APK fixture';
  await writeFile(join(f.input, apk), contents);
  await writeFile(join(f.input, 'SHA256SUMS'), `${digest(contents)}  ${apk}\n`);
  const result = await stageDownloads({ inputs: [f.input], siteDirectory: f.site, notes: f.notes });
  const checksums = await readFile(join(result.directory, 'SHA256SUMS'), 'utf8');
  const base = `/downloads/${version}/`;
  assert.deepEqual(await readSiteRelease(f.site, { version }), {
    version, androidDownloadURL: base + apk, releaseURL: base + 'release-notes.txt',
    updateAssets: [updateAsset(apk, contents), updateAsset('SHA256SUMS', checksums)],
  });
});

test('rejects an Android development APK even if the same directory includes a valid release', async t => {
  const f = await fixture(t);
  await writeFile(join(f.input, `CCBox-${version}-android-arm64-debug.apk`), 'development APK');
  await assert.rejects(stageDownloads({ inputs: [f.input], siteDirectory: f.site, notes: f.notes }), /Android development build/);
  assert.deepEqual(await readSiteRelease(f.site, { version }), { version });
});

test('an unsigned macOS package is listed only as a labelled preview', async t => {
  const root = await mkdtemp(join(tmpdir(), 'ccbox-preview-test-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const core = (await readFile(new URL('../../scripts/fetch-proxy-core.sh', import.meta.url), 'utf8')).match(/^version=(v[\d.]+)$/m)[1];
  const dmg = `CCBox-${version}-macos-arm64.dmg`;
  const assets = { [dmg]: 'dmg fixture', [`mihomo-${core}-source.tar.gz`]: 'core source', [`cage-macos-guest-${version}-source.tar.gz`]: 'guest source' };
  const input = join(root, 'macos');
  const site = join(root, 'site');
  const notes = join(root, 'notes.md');
  await mkdir(input);
  await writeFile(notes, '# CCBox\n\nmacOS preview fixture.\n');
  for (const [name, data] of Object.entries(assets)) await writeFile(join(input, name), data);
  await writeFile(join(input, 'SHA256SUMS'), Object.entries(assets).map(([name, data]) => `${digest(data)}  ${name}\n`).join(''));
  await writeFile(join(input, 'build-metadata.json'), JSON.stringify({ schema: 1, platform: 'macos', version, commitSha: 'a'.repeat(40), unsigned: true }));
  const result = await stageDownloads({ inputs: [input], siteDirectory: site, notes });
  const checksums = await readFile(join(result.directory, 'SHA256SUMS'), 'utf8');
  const base = `/downloads/${version}/`;
  assert.deepEqual(await readSiteRelease(site, { version }), {
    version, releaseURL: base + 'release-notes.txt',
    macosPreview: { version, downloadURL: base + dmg, releaseURL: base + 'release-notes.txt' },
    updateAssets: [updateAsset(dmg, assets[dmg], true), updateAsset('SHA256SUMS', checksums)],
  });
});

test('Linux packages cannot be marked unsigned', async t => {
  const f = await fixture(t);
  await writeFile(join(f.input, 'build-metadata.json'), JSON.stringify({ schema: 1, platform: 'linux', version, commitSha: 'a'.repeat(40), unsigned: true }));
  await assert.rejects(stageDownloads({ inputs: [f.input], siteDirectory: f.site, notes: f.notes }), /not allowed for linux/);
});
