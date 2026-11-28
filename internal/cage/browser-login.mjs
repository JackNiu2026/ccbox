// The browser owns these ephemeral cookies. Only the origin of a login page
// may enter the workbench tab list, including for popups and redirects.
export class LoginBrowser {
  constructor(cdp, { navigateWait = 3000 } = {}) {
    this.cdp = cdp;
    this.navigateWait = navigateWait;
    this.contexts = new Map();
  }

  tab(raw, previous) {
    if (!previous?.login && !this.contexts.has(raw.browserContextId)) return raw;
    let origin = "about:blank";
    try {
      const url = new URL(raw.url);
      if (["https:", "http:"].includes(url.protocol)) origin = url.origin;
    } catch {}
    return { ...raw, login: true, url: origin, title: "账号授权" };
  }

  async open(login, address, activate) {
    if (typeof login !== "string" || !/^[a-f0-9-]{16,64}$/.test(login)) throw new Error("登录任务无效");
    const url = new URL(address);
    if (url.protocol !== "https:" || url.username || url.password ||
        !["claude.ai", "claude.com", "console.anthropic.com", "platform.claude.com", "platform.anthropic.com"].includes(url.hostname))
      throw new Error("仅可打开官方登录地址");
    await this.finish(login);
    const { browserContextId } = await this.cdp("Target.createBrowserContext", { disposeOnDetach: true }, undefined, 12000);
    const expiry = setTimeout(() => this.close(browserContextId).catch(() => {}), 15 * 60_000);
    expiry.unref();
    this.contexts.set(browserContextId, { login, expiry });
    try {
      const { targetId } = await this.cdp("Target.createTarget", { browserContextId, url: "about:blank", newWindow: true }, undefined, 12000);
      const sessionId = await activate({ targetId, browserContextId, type: "page", url: url.href });
      const navigation = this.cdp("Page.navigate", { url: url.href }, sessionId, 30000);
      let timer;
      try {
        // Once the private tab is visible, a slow proxy must not hold up
        // the workbench or dispose of the user's authorization session.
        await Promise.race([navigation, new Promise(resolve => { timer = setTimeout(resolve, this.navigateWait); })]);
      } finally {
        clearTimeout(timer);
      }
      return { id: targetId };
    } catch {
      await this.close(browserContextId).catch(() => {});
      throw new Error("临时登录页面未能打开");
    }
  }

  async finish(login) {
    for (const [id, entry] of this.contexts) {
      if (entry.login === login) await this.close(id);
    }
  }

  async close(id) {
    const entry = this.contexts.get(id);
    if (!entry) return;
    await this.cdp("Target.disposeBrowserContext", { browserContextId: id });
    clearTimeout(entry.expiry);
    this.contexts.delete(id);
  }
}
