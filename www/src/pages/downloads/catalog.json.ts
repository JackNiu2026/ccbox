import { published } from '../../lib/release';

export const prerender = true;

export function GET() {
  const releases = published.updateAssets?.length
    ? [{ tag_name: `v${published.version}`, assets: published.updateAssets }]
    : [];
  return new Response(JSON.stringify(releases), {
    headers: { 'Content-Type': 'application/json; charset=utf-8' },
  });
}
