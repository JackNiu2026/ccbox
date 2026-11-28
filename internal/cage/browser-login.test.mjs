import assert from "node:assert/strict";
import test from "node:test";
import { LoginBrowser } from "./browser-login.mjs";

test("login redirects and popups stay private, closing one login leaves the other open", async () => {
  const closed = [];
  let contexts = 0;
  const browser = new LoginBrowser(async (method, params, sessionId) => {
    if (method === "Target.createBrowserContext") return { browserContextId: String(++contexts) };
    if (method === "Target.createTarget") {
      assert.equal(params.url, "about:blank", "login must be prepared before navigation");
      return { targetId: "tab-" + params.browserContextId };
    }
    if (method === "Page.navigate") assert.equal(sessionId, "prepared-page");
    if (method === "Target.disposeBrowserContext") closed.push(params.browserContextId);
  });
  const page = { url: "https://claude.ai/oauth/authorize?state=secret", title: "secret" };
  assert.equal(browser.tab(page), page);
  for (const id of ["a", "b"]) {
    await browser.open(id.repeat(24), page.url, async tab => {
      assert.equal(browser.tab(tab).url, "https://claude.ai");
      return "prepared-page";
    });
  }
  const popup = { ...page, targetId: "popup", browserContextId: "1", url: "http://localhost:9123/callback?code=secret" };
  const privateTab = browser.tab(popup);
  assert.equal(privateTab.title, "账号授权");
  assert.equal(privateTab.url, "http://localhost:9123");
  await browser.finish("a".repeat(24));
  assert.deepEqual(closed, ["1"]);
  assert.equal(browser.tab(popup, privateTab).url, privateTab.url);
  await browser.finish("b".repeat(24));
  assert.deepEqual(closed, ["1", "2"]);
});

test("a slow login hands off an open tab without waiting for the website", async () => {
  let finishNavigation;
  const methods = [];
  const browser = new LoginBrowser(async (method, params) => {
    methods.push(method);
    if (method === "Target.createBrowserContext") return { browserContextId: "private-context" };
    if (method === "Target.createTarget") return { targetId: "prepared-login" };
    if (method === "Page.navigate") return new Promise(resolve => { finishNavigation = resolve; });
    if (method === "Target.disposeBrowserContext") assert.equal(params.browserContextId, "private-context");
  }, { navigateWait: 10 });
  const login = "a".repeat(24);
  try {
    const deadline = AbortSignal.timeout(250);
    const timeout = new Promise((_, reject) => deadline.addEventListener("abort", () => reject(deadline.reason), { once: true }));
    const result = await Promise.race([browser.open(login, "https://claude.ai/oauth/authorize", async () => "connected"), timeout]);
    assert.deepEqual(result, { id: "prepared-login" });
    assert.equal(browser.contexts.size, 1);
    assert.ok(methods.includes("Page.navigate"));
  } finally {
    finishNavigation?.();
    await browser.finish(login);
  }
});
