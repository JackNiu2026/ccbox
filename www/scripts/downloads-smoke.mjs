import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { mkdir } from 'node:fs/promises';
import { readSiteRelease } from './stage-downloads.mjs';
const require = createRequire(import.meta.url);
const { chromium } = require(process.env.CAGE_PLAYWRIGHT_MODULE || 'playwright');
const origin = process.env.CAGE_SITE_URL || 'http://127.0.0.1:4833/';
const output = process.env.CAGE_SITE_REPORT || '_output/platform-publication/site';
const release = await readSiteRelease();
const expected = {
  macOS: release.downloadURL || release.macosPreview?.downloadURL || '/docs/installation/',
  Windows: release.windowsDownloadURL || release.windowsPreview?.downloadURL || '/docs/installation/',
  Linux: release.linuxDownloadURL || release.linuxPreview?.downloadURL || '/docs/installation/',
  Android: release.androidDownloadURL || release.androidPreview?.downloadURL || '/docs/mobile/',
  iOS: '/docs/mobile/',
};
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true, executablePath: process.env.CAGE_TEST_BROWSER_PATH });
try {
  for (const { route, width, height } of [{ route: '', width: 1280, height: 900 }, { route: '', width: 390, height: 844 }, { route: 'fr/', width: 1280, height: 900 }, { route: 'ar/', width: 390, height: 844 }]) {
    const page = await browser.newPage({ viewport: { width, height } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto(new URL(route, origin).href);
    await page.locator('[data-download-open]').first().click();
    const dialog = page.getByRole('dialog');
    await dialog.waitFor({ state: 'visible' });
    for (const [platform, target] of Object.entries(expected)) {
      const option = dialog.locator(`[data-platform="${platform}"]`);
      await option.getByRole('heading', { name: platform, exact: true }).waitFor();
      assert.equal(await option.locator('a.button').getAttribute('href'), target);
      assert.doesNotMatch(target, /github\.com|github\.io/i);
      const response = await page.request.head(new URL(target, origin).href);
      assert.equal(response.status(), 200, `${platform} destination is unavailable`);
    }
    if (!release.windowsDownloadURL && release.windowsPreview?.downloadURL) {
      const status = await dialog.locator('[data-platform="Windows"] .platform-status').textContent();
      assert.ok(status.trim() && !status.trim().startsWith('.exe'), 'unsigned Windows requires an explicit label');
      if (!route) assert.match(status, /未签名/);
    }
    const bounds = await dialog.boundingBox();
    assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= width + 1, 'download chooser overflows viewport');
    await page.screenshot({ path: `${output}/${route.replaceAll('/', '') || 'zh'}-${width}.png` });
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'hidden' });
    await page.goto(new URL(`${route}#download`, origin).href);
    await dialog.waitFor({ state: 'visible' });
    await dialog.getByRole('button').click();
    await dialog.waitFor({ state: 'hidden' });
    assert.deepEqual(errors, []);
    await page.close();
  }
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage(); await page.goto(origin);
  for (const url of new Set(Object.values(expected))) await page.locator(`noscript a[href="${url}"]`).first().waitFor({ state: 'visible' });
  await context.close();
  console.log('PASS: configured packages or local installation guides, desktop/mobile/RTL layout, modal open/close, deep link and no-JavaScript downloads');
} finally { await browser.close(); }
