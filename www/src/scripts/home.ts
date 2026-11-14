// Homepage behavior: language menu, download chooser, copy button, sticky
// header state and scroll reveals. Every feature degrades to static content.
const language = document.querySelector<HTMLDetailsElement>('#site-language');
language?.addEventListener('keydown', event => {
  if (event.key === 'Escape' && language.open) {
    event.preventDefault();
    language.open = false;
    language.querySelector('summary')?.focus();
  }
});
document.addEventListener('click', event => {
  if (language && event.target instanceof Node && !language.contains(event.target)) language.open = false;
});
const downloads = document.querySelector<HTMLDialogElement>('.download-dialog');
document.querySelectorAll<HTMLAnchorElement>('[data-download-open]').forEach(link => {
  link.addEventListener('click', event => {
    if (!downloads) return;
    event.preventDefault();
    downloads.showModal();
  });
});
downloads?.addEventListener('click', event => {
  if (event.target !== downloads) return;
  const bounds = downloads.getBoundingClientRect();
  if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) downloads.close();
});
const openDownloadHash = () => {
  if (window.location.hash === '#download' && downloads && !downloads.open) downloads.showModal();
};
window.addEventListener('hashchange', openDownloadHash);
openDownloadHash();

document.querySelectorAll<HTMLButtonElement>('[data-copy-target]').forEach(button => {
  const source = document.getElementById(button.dataset.copyTarget!);
  const label = button.querySelector('span')!;
  const original = label.textContent;
  if (!source || !navigator.clipboard) return;
  button.hidden = false;
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(source.textContent!.trim());
      label.textContent = button.dataset.copied!;
      button.classList.add('is-copied');
      setTimeout(() => { label.textContent = original; button.classList.remove('is-copied'); }, 1800);
    } catch { /* clipboard denied: the prompt stays selectable */ }
  });
});

const header = document.querySelector<HTMLElement>('.site-header');
const onScroll = () => header?.classList.toggle('is-scrolled', window.scrollY > 8);
window.addEventListener('scroll', onScroll, { passive: true });
onScroll();

const reveals = document.querySelectorAll<HTMLElement>('[data-reveal]');
if ('IntersectionObserver' in window && !matchMedia('(prefers-reduced-motion: reduce)').matches) {
  const observer = new IntersectionObserver(entries => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue;
      entry.target.classList.add('is-visible');
      observer.unobserve(entry.target);
    }
  }, { rootMargin: '0px 0px -8% 0px' });
  reveals.forEach(element => observer.observe(element));
} else {
  reveals.forEach(element => element.classList.add('is-visible'));
}
