import { useSyncExternalStore } from "react";
import languages from "./locales/languages.json";
import { setMessages, translate } from "./message-catalog.mjs";

const currentKey = "ccbox-interface-language";
const defaultLanguage = "zh-CN";
const catalogs = {
  "zh-TW": () => import("./locales/zh-TW.json"),
  en: () => import("./locales/en.json"),
};
const listeners = new Set();
let current = defaultLanguage;
let sequence = 0;

function subscribe(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export { languages, translate };

export function useInterfaceLanguage() {
  return useSyncExternalStore(subscribe, () => current);
}

export async function selectInterfaceLanguage(id, persist = true) {
  if (!languages.some(language => language.id === id)) throw new Error("不支持这个界面语言");
  const request = ++sequence;
  const catalog = id === defaultLanguage ? {} : (await catalogs[id]()).default;
  if (request !== sequence) return;
  setMessages(catalog);
  current = id;
  document.documentElement.lang = id;
  if (persist) {
    try { localStorage.setItem(currentKey, id); } catch { /* The active window still updates. */ }
  }
  listeners.forEach((listener) => listener());
}

export function restoreInterfaceLanguage() {
  let id;
  try { id = localStorage.getItem(currentKey); } catch { return; }
  if (!id) return;
  if (!languages.some(language => language.id === id)) {
    void selectInterfaceLanguage(defaultLanguage).catch(() => {});
  } else if (id !== current) {
    void selectInterfaceLanguage(id, false).catch(() => {});
  }
}
