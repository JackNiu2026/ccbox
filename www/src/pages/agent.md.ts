import guide from '../../../docs/guide/agent.md?raw';

export function GET() {
  const text = guide.replace(/^---\n[\s\S]*?\n---\n/, '').replace(/\]\(([a-z-]+)\.md(#[^)]*)?\)/g, (_, slug: string, hash = '') => "](" + import.meta.env.BASE_URL + "docs/" + (slug === "index" ? "" : slug + "/") + hash + ")");
  return new Response(text, { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
}
