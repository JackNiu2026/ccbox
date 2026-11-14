import { defineConfig } from 'astro/config';
const site = process.env.SITE_URL?.trim() || undefined;
if (site && /(?:^|\.)(?:github\.com|github\.io|githubusercontent\.com|githubassets\.com|claude-cage\.com)$/i.test(new URL(site).hostname)) {
  throw new Error('SITE_URL must point to the CCBox website.');
}
export default defineConfig({
  site,
  base: process.env.SITE_BASE || '/',
  trailingSlash: 'always',
  server: { port: 4822 },
  build: { inlineStylesheets: 'auto' },
});
