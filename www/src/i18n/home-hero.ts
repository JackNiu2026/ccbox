import type { CageLocale } from './languages';

type HeroCopy = { lead: string; emphasis: string; description: string };

export const homeHero: Record<CageLocale, HeroCopy> = {
  zh: {
    lead: '给 Claude 一个',
    emphasis: '独立工作台。',
    description: '在独立的虚拟环境中使用 Claude。终端、浏览器和编辑器集中在一个桌面工作台，项目按需共享，网络独立管理。',
  },
  'zh-TW': {
    lead: '給 Claude 一個',
    emphasis: '獨立工作台。',
    description: '在獨立的虛擬環境中使用 Claude。終端機、瀏覽器和編輯器集中在一個桌面工作台，專案按需共享，網路獨立管理。',
  },
  en: {
    lead: 'Give Claude its own',
    emphasis: 'workbench.',
    description: 'Use Claude in an isolated virtual environment. Terminal, browser and editor live in one desktop workbench; projects are shared only when you choose, and the network is managed separately.',
  },
};

type HomeUi = {
  copy: string;
  copied: string;
  /** Short headings for the four README principles, in README order. */
  principles: [string, string, string, string];
};

export const homeUi: Record<CageLocale, HomeUi> = {
  zh: { copy: '复制', copied: '已复制', principles: ['网络隔离', '环境指纹', '全链路适配', '联网检查'] },
  'zh-TW': { copy: '複製', copied: '已複製', principles: ['網路隔離', '環境指紋', '全鏈路適配', '連網檢查'] },
  en: { copy: 'Copy', copied: 'Copied', principles: ['Isolated network', 'Environment fingerprint', 'End-to-end Claude support', 'Checks before going online'] },
};
