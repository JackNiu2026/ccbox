import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { lstat, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { readSiteRelease, stageDownloads } from './stage-downloads.mjs';

const version = (await readFile(new URL('../../VERSION', import.meta.url), 'utf8')).trim();
const dependencies = JSON.parse(await readFile(new URL('../../packaging/linux/dependencies.json', import.meta.url), 'utf8'));
const installer = `ccBox-${version}-linux-amd64.deb`;
const source = dependencies.mihomoSource.file;
const digest = data => createHash('sha256').update(data).digest('hex');

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'ccbox-download-test-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const input = join(root, 'platform');
  const site = join(root, 'site');
  const notes = join(root, 'notes.md');
  await mkdir(input);
  await writeFile(notes, '# ccBox\n\nLinux release fixture.\n');
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
  assert.deepEqual(await readSiteRelease(f.site, { version }), {
    version, linuxDownloadURL: base + installer, releaseURL: base + 'release-notes.txt',
  });
  assert.equal(await readFile(join(result.directory, installer), 'utf8'), f.assets[installer]);
  assert.equal(await readFile(join(result.directory, source), 'utf8'), f.assets[source]);
  const checksums = await readFile(join(result.directory, 'SHA256SUMS'), 'utf8');
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
