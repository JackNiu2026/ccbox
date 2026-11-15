export const languages = [
  {
    code: 'zh', name: '简体中文', lang: 'zh-CN', dir: 'ltr',
    localBuildLabel: "本地自行构建", buildGuideLabel: "构建说明",
    languageLabel: '语言', downloadLabel: "下载应用", platformLabel: "选择下载平台", closeLabel: "关闭", installGuideLabel: "安装说明", unavailableLabel: "尚无公开安装包", unsignedLabel: "预览版 · 未签名",
    checksLabel: '网络与隔离检查', browserCheckLabel: '浏览器检查',
    firstLookLabel: '首次启动界面', releaseNotesLabel: '版本说明与校验文件', issuesLabel: '问题反馈',
  },
  {
    code: 'zh-TW', name: '繁體中文', lang: 'zh-TW', dir: 'ltr',
    localBuildLabel: "在本機自行建置", buildGuideLabel: "建置說明",
    languageLabel: '語言', downloadLabel: "下載應用程式", platformLabel: "選擇下載平台", closeLabel: "關閉", installGuideLabel: "安裝說明", unavailableLabel: "尚無公開安裝包", unsignedLabel: "預覽版 · 未簽章",
    checksLabel: '網路與隔離檢查', browserCheckLabel: '瀏覽器檢查',
    firstLookLabel: '首次啟動畫面', releaseNotesLabel: '版本說明與校驗檔案', issuesLabel: '問題回報',
  },
  {
    code: 'en', name: 'English', lang: 'en', dir: 'ltr',
    localBuildLabel: "Build it yourself", buildGuideLabel: "Build guide",
    languageLabel: 'Language', downloadLabel: "Download", platformLabel: "Choose your platform", closeLabel: "Close", installGuideLabel: "Installation guide", unavailableLabel: "No public installer yet", unsignedLabel: "Preview · unsigned",
    checksLabel: 'Network and isolation checks', browserCheckLabel: 'Browser check',
    firstLookLabel: 'First launch screen', releaseNotesLabel: 'Release notes and checksums', issuesLabel: 'Report an issue',
  },
] as const;

export type CageLocale = typeof languages[number]['code'];
