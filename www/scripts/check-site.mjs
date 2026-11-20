// Check emitted pages, not just the locale configuration: catch missing routes,
// broken project subpaths and languages drifting away from the README menu.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { readSiteRelease, upstreamReleaseURL } from './stage-downloads.mjs';
const root = fileURLToPath(new URL('../../', import.meta.url));
const readme = fs.readFileSync(path.join(root,'README.md'),'utf8');
const menu = readme.split('\n').find(line => line.includes('(README.md)') && line.includes('(README.'));
assert.ok(menu, 'README language navigation is missing');
const codes = [...menu.matchAll(/\]\(README(?:\.([\w-]+))?\.md\)/g)].map(m => m[1] || 'zh');
// README.md has no dot before the base name's extension in the optional group.
assert.equal(codes[0], 'zh');
assert.deepEqual(codes, ['zh', 'zh-TW', 'en'], 'The site offers Simplified Chinese, Traditional Chinese and English');
const dist = path.join(root,'www/dist');
const siteURL = process.env.SITE_URL?.trim();
const site = siteURL ? new URL(siteURL) : undefined;
const measurementId = process.env.PUBLIC_GA_MEASUREMENT_ID?.trim();
const brandMark = fs.readFileSync(path.join(dist, 'ccbox-icon.svg'), 'utf8');
assert.match(brandMark, /<path\b/, 'Published logo must contain vector geometry');
assert.doesNotMatch(brandMark, /<image\b|data:image\//i, 'Published logo must not embed a raster image');
const htmlFiles = fs.readdirSync(dist, { recursive: true }).filter(file => file.endsWith('.html'));
for (const file of htmlFiles) {
  const html = fs.readFileSync(path.join(dist, file), 'utf8');
  assert.match(html, /<link\b[^>]*\brel="icon"[^>]*\bhref="\/ccbox-icon\.svg"/, `${file}: vector favicon is missing`);
  assert.match(html, /<header class="site-header">[\s\S]*?<img\b[^>]*\bsrc="\/ccbox-icon\.svg"/, `${file}: header logo is missing`);
  assert.doesNotMatch(html, /<img\b[^>]*\bsrc="\/ccbox-icon\.png"/, `${file}: visible logo should use SVG`);
  assert.doesNotMatch(html, upstreamReleaseURL, file + ": public page links to an upstream service");
  for (const [, href] of html.matchAll(/<a\b[^>]*\bhref="((?:https?:)?\/\/[^"]+)"/gi)) {
    const destination = new URL(href, 'https://ccbox.org/');
    assert.equal(destination.origin, 'https://ccbox.org', `${file}: public page links outside CCBox`);
  }
  assert.ok(!/\b(?:Claude Cage|Lee-xeo|GetToken)\b/i.test(html), `${file}: legacy brand appears in a public page`);
  const homepageLocale = codes.find(code => file === (code === 'zh' ? 'index.html' : path.join(code, 'index.html')));
  if (homepageLocale) {
    const docsTitle = { zh: '使用文档', 'zh-TW': '使用文件', en: 'User guide' }[homepageLocale];
    assert.ok(html.includes('<h2 id="docs-title">' + docsTitle + '</h2>'), file + ': user guide heading is missing');
    assert.doesNotMatch(html, /<section[^>]*aria-labelledby="license-title"/, file + ': source and license module is still present');
    assert.doesNotMatch(html, /href="\/LICENSE\.txt"|opensource\.org\/licenses\/MIT/i, file + ': homepage exposes a license link');
  }
  assert.ok(!/(?:src|href)="\/Claude-Cage(?:\/|")/.test(html), `${file}: old project base remains`);
  if (file !== '404.html') {
    const route = file.replaceAll(path.sep, '/').replace(/index\.html$/, '');
    if (site) {
      assert.ok(html.includes(`rel="canonical" href="${new URL(route, site)}"`), file + ": incorrect canonical URL");
    } else {
      assert.ok(!html.includes('rel="canonical"'), file + ": no canonical origin is configured");
    }
  }
  if (file !== path.join('privacy', 'index.html')) {
    assert.ok(html.includes('href="/privacy/"'), `${file}: analytics disclosure link is missing`);
  }
  const tags = [...html.matchAll(/<script\b[^>]*src="https:\/\/www\.googletagmanager\.com\/gtag\/js\?id=([^"]+)"[^>]*>/g)];
  assert.equal(tags.length, measurementId ? 1 : 0, `${file}: expected one analytics tag only when configured`);
  if (measurementId) {
    assert.equal(tags[0][1], measurementId, `${file}: incorrect analytics destination`);
    assert.ok(/\sasync(?:\s|>)/.test(tags[0][0]), `${file}: analytics must not block page rendering`);
  }
}
assert.equal(fs.readFileSync(path.join(dist, 'LICENSE.txt'), 'utf8'), fs.readFileSync(path.join(root, 'LICENSE'), 'utf8'), 'Public MIT license must preserve the original copyright notice');
const published = await readSiteRelease(path.join(root, 'www'));
const version = fs.readFileSync(path.join(root, 'VERSION'), 'utf8').trim();
assert.equal(published.version, version, 'Website release version must match VERSION');
const downloadNames = {
  downloadURL: `CCBox-${version}-macos-arm64.dmg`,
  windowsDownloadURL: `CCBox-${version}-windows-amd64-setup.exe`,
  linuxDownloadURL: `CCBox-${version}-linux-amd64.deb`,
  androidDownloadURL: `CCBox-${version}-android-arm64.apk`,
};
for (const [field, name] of Object.entries(downloadNames)) {
  if (published[field]) assert.equal(published[field], `/downloads/${version}/${name}`);
}
const expectedDownloads = Object.entries(downloadNames).filter(([field]) => published[field]).map(([, name]) => name);
const legacyPreviewNames = {
  macosPreview: `CCBox-${version}-macos-arm64-test.dmg`,
  windowsPreview: `CCBox-${version}-windows-amd64-setup-unsigned.exe`,
  linuxPreview: `CCBox-${version}-linux-amd64.deb`,
  androidPreview: `CCBox-${version}-android-arm64-debug.apk`,
};
const previewReleaseFields = {
  macosPreview: 'downloadURL',
  windowsPreview: 'windowsDownloadURL',
  linuxPreview: 'linuxDownloadURL',
  androidPreview: 'androidDownloadURL',
};
const releaseBase = `/downloads/${version}/`;
const previewLinks = Object.entries(legacyPreviewNames).filter(([field]) => published[field]);
for (const [field, legacyName] of previewLinks) {
  const preview = published[field];
  const releaseField = previewReleaseFields[field];
  const legacy = preview.releaseURL === releaseBase + 'preview-notes.txt';
  assert.equal(preview.version, version);
  assert.ok(!published[releaseField], `${field}: preview and official installer are both listed`);
  assert.ok(legacy || field === 'macosPreview' || field === 'windowsPreview', `${field}: unsupported release preview`);
  const name = legacy ? legacyName : downloadNames[releaseField];
  const notesName = legacy ? 'preview-notes.txt' : 'release-notes.txt';
  assert.equal(preview.downloadURL, releaseBase + name);
  assert.equal(preview.releaseURL, releaseBase + notesName);
  expectedDownloads.push(name);
}
if (expectedDownloads.length) {
  const folder = path.join(dist, 'downloads', version);
  const entries = new Set();
  for (const line of fs.readFileSync(path.join(folder, 'SHA256SUMS'), 'utf8').trim().split('\n')) {
    const match = /^([a-f0-9]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*)$/.exec(line.trimEnd());
    assert.ok(match && !entries.has(match[2]), 'Malformed or duplicate website checksum entry');
    const [, expectedHash, name] = match;
    const hash = createHash('sha256');
    for await (const chunk of fs.createReadStream(path.join(folder, name))) hash.update(chunk);
    assert.equal(hash.digest('hex'), expectedHash, `Website download checksum mismatch: ${name}`);
    entries.add(name);
  }
  for (const name of expectedDownloads) assert.ok(entries.has(name), `Website checksum missing: ${name}`);
  const availableInstallers = [...entries].filter(name => /\.(dmg|exe|deb|apk)$/i.test(name)).sort();
  assert.deepEqual(availableInstallers, expectedDownloads.sort(), 'Website contains an unmapped or unofficial installer');
}
for (const [field] of previewLinks) {
  const previewNotes = fs.readFileSync(path.join(dist, published[field].releaseURL.slice(1)), 'utf8');
  assert.doesNotMatch(previewNotes, upstreamReleaseURL, 'Published preview notes link to an upstream service');
}
if (published.releaseURL) {
  const releaseNotes = fs.readFileSync(path.join(dist, published.releaseURL.slice(1)), 'utf8');
  assert.doesNotMatch(releaseNotes, upstreamReleaseURL, 'Published release notes link to an upstream service');
}
for (const code of codes) {
  const html = fs.readFileSync(path.join(dist,code === 'zh' ? '' : code,'index.html'),'utf8');
  const picker = html.match(/<details\b[^>]*id="site-language"[^>]*>([\s\S]*?)<\/details>/)?.[1];
  assert.ok(picker, `${code}: missing language picker`);
  const options = [...picker.matchAll(/<a\b[^>]*href="([^"]+)"/g)].map(m=>m[1]);
  assert.equal(options.length,codes.length, `${code}: language dropdown missing options`);
  assert.deepEqual(options, codes.map(locale => `/${locale === 'zh' ? '' : locale + '/'}`), `${code}: incorrect language destinations`);
  assert.equal([...picker.matchAll(/aria-current="page"/g)].length, 1, `${code}: current language must be marked once`);
  assert.ok(!html.includes('<select'), `${code}: system popup selector remains`);
  assert.ok(html.includes('id="site-language"'), `${code}: missing language selector`);
  assert.ok(html.includes(`dir="${['ar','fa','ps','prs'].includes(code)?'rtl':'ltr'}"`), `${code}: incorrect reading direction`);
  for (const match of html.matchAll(/(?:src|href)="(\/[^"#]+)"/g)) {
    const relative = decodeURIComponent(match[1].slice(1));
    const target = path.join(dist,relative.endsWith('/') ? relative+'index.html' : relative);
    assert.ok(fs.existsSync(target), `${code}: broken local asset or link ${match[1]}`);
  }
  assert.ok(!html.includes('/Users/'), `${code}: local developer path leaked`);
  if (published.downloadURL) assert.ok(html.includes(`href="${published.downloadURL}"`), code + ": macOS download is missing");
  const downloadSection = html.match(/<section\b[^>]*id="download"[^>]*>([\s\S]*?)<\/section>/)?.[1];
  assert.ok(downloadSection?.includes(`CCBox · v${published.version}`), `${code}: download release label must follow published metadata`);
  assert.ok(!downloadSection?.includes('CLAUDE CAGE · ALPHA'), `${code}: stale release channel label`);
  assert.equal([...html.matchAll(/<a\b[^>]*\bdata-download-open(?:\s|>)/g)].length, 3, `${code}: all download entries must open the platform chooser`);
  assert.ok(html.includes('aria-labelledby="platform-title"'), `${code}: platform dialog has no accessible title`);
  for (const platform of ['macOS', 'Windows', 'Linux', 'Android', 'iOS']) {
    assert.ok(html.includes(`data-platform="${platform}"`), `${code}: missing ${platform} choice`);
  }
  const platformURLs = [
    ['macOS', published.downloadURL || published.macosPreview?.downloadURL],
    ['Windows', published.windowsDownloadURL || published.windowsPreview?.downloadURL],
    ['Linux', published.linuxDownloadURL || published.linuxPreview?.downloadURL],
    ['Android', published.androidDownloadURL || published.androidPreview?.downloadURL],
  ];
  const installerLinks = [...html.matchAll(/href="([^"]+\.(?:dmg|exe|deb|apk))"/g)].map(match => match[1]);
  for (const [platform, url] of platformURLs) {
    if (!url) continue;
    assert.equal(installerLinks.filter(link => link === url).length, 2, `${code}: ${platform} installer must appear in its platform card and fallback`);
  }
  assert.equal(installerLinks.length, 2 * platformURLs.filter(([, url]) => url).length, `${code}: unexpected installer link`);
  for (const [platform, field] of [['macOS', 'macosPreview'], ['Windows', 'windowsPreview'], ['Linux', 'linuxPreview'], ['Android', 'androidPreview']]) {
    if (!published[field]) continue;
    const card = html.match(new RegExp(`<article\\b[^>]*data-platform="${platform}"[^>]*>([\\s\\S]*?)<\\/article>`))?.[1];
    assert.ok(card?.includes('data-preview'), `${code}: ${platform} preview status is missing`);
    assert.ok(card?.includes(`href="${published[field].downloadURL}"`), `${code}: ${platform} preview download is missing`);
    assert.ok(card?.includes(`href="${published[field].releaseURL}"`), `${code}: ${platform} preview notes are missing`);
  }
  const ios = html.match(/<article\b[^>]*data-platform="iOS"[^>]*>([\s\S]*?)<\/article>/)?.[1];
  assert.ok(ios?.includes('href="/docs/mobile/"'), `${code}: iOS needs its local build guide`);
  assert.ok(ios?.includes('/platforms/apple.svg'), `${code}: iOS platform icon is missing`);
  const android = html.match(/<article\b[^>]*data-platform="Android"[^>]*>([\s\S]*?)<\/article>/)?.[1];
  assert.ok(android?.includes('/platforms/android.svg'), `${code}: Android platform icon is missing`);
}
for (const retired of ['ru', 'ar', 'fa', 'ko', 'es', 'my', 'be', 'ps', 'prs', 'am', 'so', 'ti', 'fr']) {
  assert.ok(!fs.existsSync(path.join(dist, retired, 'index.html')), `${retired}: retired language page still published`);
}
console.log(`Verified ${codes.length} README languages, dropdowns, directions and local assets.`);

// Check actual guide pages and every cross-page anchor, including project bases.
const guideDir = path.join(dist, 'docs');
const guideFiles = fs.readdirSync(guideDir, { recursive: true }).filter(file => file.endsWith('index.html'));
assert.ok(guideFiles.length >= 12, 'User guide chapters are missing');
const guideSourceFiles = fs.readdirSync(path.join(root, 'docs/guide')).filter(file => file.endsWith('.md'));
assert.equal(guideFiles.length, guideSourceFiles.length, 'Guide source and routes have drifted');
for (const relative of guideFiles) {
  const filename = path.join(guideDir, relative);
  const html = fs.readFileSync(filename, 'utf8');
  assert.ok(html.includes('id="guide-search"'), `${relative}: missing search`);
  assert.ok(html.includes('id="guide-article"'), `${relative}: missing article`);
  assert.ok(html.includes('aria-label="前后章节"'), `${relative}: missing chapter navigation`);
  assert.ok(!html.includes('/Users/'), `${relative}: local developer path leaked`);
  assert.ok(!html.includes('<select'), `${relative}: system popup selector remains`);
  assert.equal([...html.matchAll(/<h1\b/g)].length, 1, `${relative}: expected one page title`);
  const current = `/docs/${relative.replace(/index\.html$/, '').replaceAll(path.sep, '/')}`;
  const pageURL = new URL(current, site ?? 'https://example.test');
  for (const [, reference] of html.matchAll(/<a\b[^>]*\bhref="([^"]+)"/gi)) {
    const href = reference.replaceAll('&amp;', '&');
    const url = new URL(href, pageURL);
    if (url.origin !== pageURL.origin) continue;
    const local = decodeURIComponent(url.pathname.slice(1));
    const target = path.join(dist, !local || local.endsWith('/') ? local + 'index.html' : local);
    assert.ok(fs.existsSync(target), `${relative}: broken guide link ${href}`);
    if (url.hash) {
      const targetHTML = fs.readFileSync(target, 'utf8');
      const id = decodeURIComponent(url.hash.slice(1));
      assert.ok(targetHTML.includes(`id="${id}"`), `${relative}: missing anchor ${href}`);
    }
  }
}
for (const code of codes) {
  const html = fs.readFileSync(path.join(dist, code === 'zh' ? '' : code, 'index.html'), 'utf8');
  assert.ok(html.includes('href="/docs/"'), `${code}: homepage needs a guide entry`);
}
console.log(`Verified ${guideFiles.length} user guide pages, local links, anchors, search and homepage entries.`);

const agentGuide = fs.readFileSync(path.join(dist, 'agent.md'), 'utf8');
assert.equal(agentGuide, fs.readFileSync(path.join(dist, 'llms-full.txt'), 'utf8'), 'Agent text copies must share the same source');
assert.ok(agentGuide.includes('agent capabilities') && agentGuide.includes('job_not_found'), 'Agent guide lacks discovery or recovery contract');
assert.ok(!agentGuide.includes('title: 让 Agent'), 'Agent Markdown must omit frontmatter');
for (const name of ["agent.md", "llms.txt", "llms-full.txt"]) {
  const text = fs.readFileSync(path.join(dist, name), "utf8");
  assert.doesNotMatch(text, /github\.com|github\.io|claude-cage\.com/i, name + ": upstream service in published guide");
}
for (const name of ["install.sh", "cage-install.sh", "agent-bootstrap.mjs", "first-run-2026-11.jpg", "workbench-2026-11.jpg"]) {
  assert.ok(!fs.existsSync(path.join(dist, name)), name + ": retired upstream asset was published");
}
assert.ok(fs.readFileSync(path.join(dist, "docs/agent/index.html"), "utf8").includes("JSON 结果和异步任务"), "Rendered Agent guide missing");
const home = fs.readFileSync(path.join(dist, "index.html"), "utf8");
for (const name of ["workbench-preview.jpg", "first-launch-preview.jpg"]) {
  assert.ok(fs.existsSync(path.join(dist, name)), name + ": preview missing");
  assert.ok(home.includes(`src="/${name}"`), name + ": homepage preview missing");
}
assert.ok(home.includes('class="first-look"') && home.includes('class="evidence-grid"'), "Homepage preview sections missing");
assert.match(home, /src="[^"]*browser-checks[^"]*"/, "CCBox browser-check preview missing");
assert.doesNotMatch(home, /community-browser-check|FuckClaude/i, "Upstream browser-check material remains");
for (const file of ["docs/index.html", "docs/workbench/index.html"]) {
  assert.ok(fs.readFileSync(path.join(dist, file), "utf8").includes('src="/first-launch-preview.jpg"'), file + ": startup preview missing");
}
console.log("Verified homepage preview sections, Agent guides, and absence of upstream routes.");
