import { isIP } from "node:net";
import { closeSync, constants, fstatSync, openSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { randomUUID } from "node:crypto";

const maxAddress = 1024;
const maxEntries = 300;
const trackingParams = /^(utm_(source|medium|campaign|term|content|id)|fbclid|gclid)$/i;
const authSegment = /^(accounts?|auth(?:entication|orize)?|callback|consent|login|log-in|logon|oauth\d*|oidc|password|register|reset(?:-password)?|saml\d*|signin|sign-in|signup|sign-up|sso|token|verify)$/i;
const internalNames = new Set(["internal", "lan", "local", "localhost", "home", "onion"]);

export function historyAddress(value) {
  if (typeof value !== "string" || value.length > maxAddress) return "";
  try {
    const url = new URL(value);
    const labels = url.hostname.replace(/\.$/, "").split(".");
    if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.port ||
        url.hash || labels.length < 2 || isIP(url.hostname) || internalNames.has(labels.at(-1)) ||
        labels.slice(0, -2).some(label => authSegment.test(label)) ||
        [...url.searchParams.keys()].some(key => !trackingParams.test(key))) return "";
    for (const segment of url.pathname.split("/")) {
      const decoded = decodeURIComponent(segment);
      if (authSegment.test(decoded) || /[%/\\;=\u0000-\u001f]/u.test(decoded)) return "";
    }
    url.search = "";
    return url.href;
  } catch {
    return "";
  }
}

export function ordinaryHistoryTab(tab, context) {
  return context !== null && tab?.type === "page" && !tab.login && !tab.incognito &&
    (!tab.kind || tab.kind === "web") && (tab.browserContextId || "") === context;
}

export class BrowserHistory {
  #items = [];
  #writable = true;

  constructor(file, limit = maxEntries) {
    this.file = file;
    this.limit = limit;
    try {
      const fd = openSync(file, constants.O_RDONLY | (constants.O_NOFOLLOW || 0));
      try {
        const info = fstatSync(fd);
        if (!info.isFile() || info.size > (maxAddress + 8) * maxEntries) throw new Error("Invalid browser history");
        const addresses = JSON.parse(readFileSync(fd, "utf8"));
        if (!Array.isArray(addresses)) throw new Error("Invalid browser history");
        this.#items = [...new Set(addresses.map(historyAddress).filter(Boolean))].slice(0, limit);
      } finally {
        closeSync(fd);
      }
    } catch (error) {
      this.#writable = error.code === "ENOENT";
    }
  }

  entries() { return [...this.#items]; }

  visit(tab, frame, context = "") {
    if (!this.#writable || !ordinaryHistoryTab(tab, context) || !frame || frame.parentId || frame.unreachableUrl) return;
    const address = historyAddress(frame.url);
    if (!address || address === this.#items[0]) return;
    const items = [address, ...this.#items.filter(item => item !== address)].slice(0, this.limit);
    try { this.#save(items); } catch { /* Browsing remains available if the history disk cannot be written. */ }
  }

  clear() {
    this.#save([]);
    this.#writable = true;
  }

  #save(items) {
    const temporary = this.file + "." + randomUUID();
    try {
      writeFileSync(temporary, JSON.stringify(items), { flag: "wx", mode: 0o600 });
      renameSync(temporary, this.file);
      this.#items = items;
    } finally {
      rmSync(temporary, { force: true });
    }
  }
}
