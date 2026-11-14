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
  ru: {
    lead: 'Claude в отдельном',
    emphasis: 'рабочем пространстве.',
    description: 'Запускайте Claude в изолированной виртуальной среде с терминалом, браузером и редактором в одном окне.',
  },
  ar: {
    lead: 'مساحة عمل مستقلة',
    emphasis: 'لـ Claude.',
    description: 'استخدم Claude في بيئة افتراضية معزولة تجمع الطرفية والمتصفح والمحرر في نافذة واحدة.',
  },
  fa: {
    lead: 'فضای کاری مستقل',
    emphasis: 'برای Claude.',
    description: 'از Claude در یک محیط مجازی ایزوله استفاده کنید و پایانه، مرورگر و ویرایشگر را در یک پنجره داشته باشید.',
  },
  ko: {
    lead: 'Claude를 위한',
    emphasis: '독립 작업 공간.',
    description: '격리된 가상 환경에서 Claude를 사용하세요. 터미널, 브라우저, 편집기를 하나의 창에서 이용할 수 있습니다.',
  },
  es: {
    lead: 'Claude en su propio',
    emphasis: 'espacio de trabajo.',
    description: 'Usa Claude en un entorno virtual aislado con terminal, navegador y editor en una sola ventana.',
  },
  my: {
    lead: 'Claude အတွက်',
    emphasis: 'သီးခြား အလုပ်နေရာ။',
    description: 'သီးခြား virtual environment ထဲမှာ Claude ကို အသုံးပြုပြီး terminal၊ browser နဲ့ editor ကို ဝင်းဒိုးတစ်ခုတည်းမှာ သုံးနိုင်ပါတယ်။',
  },
  be: {
    lead: 'Claude у асобнай',
    emphasis: 'працоўнай прасторы.',
    description: 'Працуйце з Claude у ізаляваным віртуальным асяроддзі з тэрміналам, браўзерам і рэдактарам у адным акне.',
  },
  ps: {
    lead: 'د Claude لپاره',
    emphasis: 'خپلواک کاري ځای.',
    description: 'Claude په یوه جلا مجازي چاپېریال کې وکاروئ چې ټرمینل، براوزر او اېډېټر په یوه کړکۍ کې راټولوي.',
  },
  prs: {
    lead: 'فضای کاری مستقل',
    emphasis: 'برای Claude.',
    description: 'Claude را در یک محیط مجازی جداگانه استفاده کنید و ترمینل، مرورگر و ویرایشگر را در یک پنجره داشته باشید.',
  },
  am: {
    lead: 'ለ Claude',
    emphasis: 'የተለየ የሥራ ቦታ።',
    description: 'Claudeን በተነጠለ ምናባዊ አካባቢ ይጠቀሙ። ተርሚናል፣ አሳሽ እና አርታዒ በአንድ መስኮት ይገኛሉ።',
  },
  so: {
    lead: 'Claude oo leh',
    emphasis: 'goob shaqo u gaar ah.',
    description: 'Ku isticmaal Claude deegaan dalwad ah oo gooni u ah, adigoo terminalka, biraawsarka iyo tifaftiraha ku wada adeegsanaya hal daaqad.',
  },
  ti: {
    lead: 'ን Claude',
    emphasis: 'ፍሉይ ቦታ ስራሕ።',
    description: 'Claude ኣብ ዝተነጸለ ምናባዊ ከባቢ ተጠቐሙ። ተርሚናል፣ መርበብ መዳህሰሲን መአረሚን ኣብ ሓደ መስኮት ይርከቡ።',
  },
  fr: {
    lead: 'Claude dans son propre',
    emphasis: 'espace de travail.',
    description: 'Utilisez Claude dans un environnement virtuel isolé qui réunit terminal, navigateur et éditeur dans une même fenêtre.',
  },
};

type HomeUi = { copy: string; copied: string };

export const homeUi: Record<CageLocale, HomeUi> = {
  zh: { copy: '复制', copied: '已复制' },
  'zh-TW': { copy: '複製', copied: '已複製' },
  ru: { copy: 'Копировать', copied: 'Скопировано' },
  ar: { copy: 'نسخ', copied: 'تم النسخ' },
  fa: { copy: 'کپی', copied: 'کپی شد' },
  ko: { copy: '복사', copied: '복사됨' },
  es: { copy: 'Copiar', copied: 'Copiado' },
  my: { copy: 'ကူးယူရန်', copied: 'ကူးယူပြီး' },
  be: { copy: 'Скапіяваць', copied: 'Скапіявана' },
  ps: { copy: 'کاپي', copied: 'کاپي شو' },
  prs: { copy: 'کاپی', copied: 'کاپی شد' },
  am: { copy: 'ቅዳ', copied: 'ተቀድቷል' },
  so: { copy: 'Koobiyee', copied: 'Waa la koobiyeeyay' },
  ti: { copy: 'ቅዳሕ', copied: 'ተቐዲሑ' },
  fr: { copy: 'Copier', copied: 'Copié' },
};
