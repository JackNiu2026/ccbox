import { cageCopy, type CageLocale } from '../i18n/cage';
import { homeHero, homeUi } from '../i18n/home-hero';
import { platformsFor, published } from './release';

// The user guide is written in Chinese; other languages say so in the label.
const guideLabels: Record<CageLocale, string> = {
  zh: '使用文档', 'zh-TW': '使用文件 · 中文', en: 'User guide · Chinese',
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
