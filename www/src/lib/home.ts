import { cageCopy, type CageLocale } from '../i18n/cage';
import { homeHero, homeUi } from '../i18n/home-hero';
import { platformsFor, published } from './release';

const guideLabels: Record<CageLocale, string> = {
  zh: '使用文档', 'zh-TW': '使用文件 · 中文', ru: 'Документация · 中文', ar: 'الدليل · 中文', fa: 'راهنما · 中文',
  ko: '사용 문서 · 中文', es: 'Documentación · 中文', my: 'လမ်းညွှန် · 中文', be: 'Дакументацыя · 中文',
  ps: 'لارښود · 中文', prs: 'راهنما · 中文', am: 'መመሪያ · 中文', so: 'Hagaha · 中文', ti: 'መምርሒ · 中文', fr: 'Documentation · 中文',
};

export const pathFor = (code: CageLocale) => `${import.meta.env.BASE_URL}${code === 'zh' ? '' : `${code}/`}`;
export const docHref = (slug: string) => `${import.meta.env.BASE_URL}docs/${slug === 'index' ? '' : `${slug}/`}`;

/** Everything a homepage section needs for one locale. */
export function homeContext(locale: CageLocale) {
  const base = import.meta.env.BASE_URL;
  const copy = cageCopy[locale];
  return {
    locale, base, copy, published,
    hero: homeHero[locale],
    ui: homeUi[locale],
    guideLabel: guideLabels[locale],
    platforms: platformsFor(locale, copy.unsignedLabel, base),
  };
}

export type HomeContext = ReturnType<typeof homeContext>;
