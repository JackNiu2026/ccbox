import assert from "node:assert/strict";
import { createWorkbench } from "./workbench-fixture.mjs";

let quits = 0;
const { browser, page, errors } = await createWorkbench({
  waitForReady: false,
  clientContext: {
    mode: "local", desktop_gateway: true, connection_id: "local",
    client_platform: "windows", server_platform: "windows",
    connection: { state: "connected" },
  },
  nativeRequest: async ({ action }) => {
    if (action === "quitNow") quits++;
    return action === "accountDevice" ? { supported: false } : null;
  },
  beforeNavigate: async (_page, state) => {
    Object.assign(state.account, { unlocked: false, online_enabled: true });
  },
});
try {
  for (const width of [960, 1280, 650]) {
    await page.setViewportSize({ width, height: 660 });
    const entry = page.locator(".account-entry");
    const quit = page.getByRole("button", { name: "退出 CCBox", exact: true });
    await quit.waitFor();
    await quit.scrollIntoViewIfNeeded();
    const entryBox = await entry.boundingBox();
    const quitBox = await quit.boundingBox();
    assert.ok(quitBox.x >= entryBox.x && quitBox.x + quitBox.width <= entryBox.x + entryBox.width + 1,
      `exit control is outside the account column at ${width}px`);
    assert.equal(await entry.getByRole("button", { name: "退出 CCBox", exact: true }).count(), 1);
  }
  await page.getByRole("button", { name: "退出 CCBox", exact: true }).click();
  await page.waitForFunction(() => !document.querySelector(".account-exit-app:disabled"));
  assert.equal(quits, 1);
  assert.deepEqual(errors, []);
  console.log("PASS exit control in the account column at wide and narrow sizes");
} finally {
  await browser.close();
}
