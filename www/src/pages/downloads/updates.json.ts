export const prerender = true;

// The update service overrides this exact path in production. A static-only
// deployment must never publish a client update just by uploading packages.
export function GET() {
  return new Response('[]', {
    headers: { 'Content-Type': 'application/json; charset=utf-8' },
  });
}
