import { randomUUID } from 'node:crypto';
import { mkdir, readFile, rename, rmdir, unlink, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';
import { prepareReleaseAssets } from '../../scripts/prepare-release-assets.mjs';

const site = fileURLToPath(new URL('../', import.meta.url));
const metadataFile = siteDirectory => join(siteDirectory, 'src', 'data', 'local-release.json');
export const upstreamReleaseURL = /(?:https?:)?\/\/(?:[\w-]+\.)*(?:github\.com|github\.io|github\.dev|githubusercontent\.com|githubassets\.com|git\.io|gh\.io|claude-cage\.com|fuck-claude\.vercel\.app)\b/i;

export async function readSiteRelease(siteDirectory = site, baseline) {
  try {
    return JSON.parse(await readFile(metadataFile(siteDirectory), 'utf8'));
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
    return baseline ?? JSON.parse(await readFile(join(siteDirectory, 'src', 'data', 'release.json'), 'utf8'));
  }
}

export async function stageDownloads({ inputs, notes, siteDirectory = site }) {
  const version = (await readFile(new URL('../../VERSION', import.meta.url), 'utf8')).trim();
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) throw new Error('Invalid repository VERSION');
  if (!notes) throw new Error('Provide the ccBox release notes with --notes');
  const releaseNotes = (await readFile(notes, 'utf8')).trim();
  if (!releaseNotes) throw new Error('Release notes must not be empty');
  if (upstreamReleaseURL.test(releaseNotes)) throw new Error('Release notes contain an upstream URL');

  const output = join(siteDirectory, 'public', 'downloads', version);
  const manifest = metadataFile(siteDirectory);
  await mkdir(dirname(manifest), { recursive: true });
  const result = await prepareReleaseAssets({ inputs, output });
  const notesName = 'release-notes.txt';
  const temporaryManifest = join(dirname(manifest), `.local-release-${randomUUID()}.tmp`);
  const base = `/downloads/${version}/`;
  try {
    const sums = await readFile(join(output, 'SHA256SUMS'), 'utf8');
    const metadata = { version, releaseURL: base + notesName };
    const installers = {
      downloadURL: `ccBox-${version}-macos-arm64.dmg`,
      windowsDownloadURL: `ccBox-${version}-windows-amd64-setup.exe`,
      linuxDownloadURL: `ccBox-${version}-linux-amd64.deb`,
      androidDownloadURL: `ccBox-${version}-android-arm64.apk`,
    };
    for (const [key, name] of Object.entries(installers)) {
      if (result.assets.includes(name)) metadata[key] = base + name;
    }
    const listing = result.assets.map(name => base + name).join('\n');
    await writeFile(join(output, notesName), `${releaseNotes}\n\n${listing}\n${base}SHA256SUMS\n\n${sums}`, { flag: 'wx' });
    await writeFile(temporaryManifest, `${JSON.stringify(metadata, null, 2)}\n`, { flag: 'wx' });
    await rename(temporaryManifest, manifest);
    return result;
  } catch (error) {
    await unlink(temporaryManifest).catch(() => {});
    await Promise.all([...result.assets, 'SHA256SUMS', notesName].map(name => unlink(join(output, name)).catch(() => {})));
    await rmdir(output).catch(() => {});
    throw error;
  }
}

async function main() {
  const { values, positionals } = parseArgs({ allowPositionals: true, options: {
    notes: { type: 'string' }, help: { type: 'boolean', short: 'h' },
  } });
  if (values.help) {
    console.log('Usage: node www/scripts/stage-downloads.mjs --notes FILE PLATFORM_RELEASE_DIRECTORY [...]');
    return;
  }
  const result = await stageDownloads({ inputs: positionals, notes: values.notes });
  console.log(`Staged ccBox ${result.version} downloads in ${result.directory}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
