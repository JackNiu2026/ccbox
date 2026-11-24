import assert from "node:assert/strict";
import { createWorkbench } from "./workbench-fixture.mjs";

const terminalId = "0123456789abcdef";
const notificationCalls = [];
let permission = "not_determined";
const status = () => ({ supported: true, permission, canOpenSettings: true, sound: true });
const { browser, page, errors } = await createWorkbench({
  waitForReady: false,
  nativeRequest: async body => {
    if (!/notification/i.test(body.action)) return null;
    notificationCalls.push(body.action);
    if (body.action === "requestNotificationPermission") permission = "authorized";
    if (body.action === "showNotification") return { delivered: permission === "authorized" };
    return status();
  },
  beforeNavigate: async (page, state) => {
    state.status.ready = true;
    state.status.vm_running = true;
    state.terminals = [{ id: terminalId, title: "测试终端", kind: "shell", workspace: state.settings.workspace }];
    await page.route("**/api/desktop-license", route => route.fulfill({ json: { state: "active" } }));
    await page.route("**/api/client/notifications", route => route.fulfill({
      json: { turn_completion: "off", approval: "off", sound: true },
    }));
  },
});

try {
  await page.getByRole("button", { name: /^设置/ }).click();
  await page.getByRole("button", { name: "终端通知", exact: true }).click();
  await page.getByRole("button", { name: "发送测试通知" }).click();
  await page.getByText("已发送测试通知。", { exact: true }).waitFor();
  assert.ok(notificationCalls.includes("requestNotificationPermission"));
  assert.ok(notificationCalls.indexOf("requestNotificationPermission") < notificationCalls.indexOf("showNotification"));

  await page.getByRole("button", { name: "账户", exact: true }).click();
  const username = page.getByRole("textbox", { name: "用户名", exact: true });
  await username.fill("未保存资料");
  await page.evaluate(id => {
    window.dispatchEvent(new CustomEvent("cage-notification-click", {
      detail: { terminal_id: id, connection_id: "local" },
    }));
  }, terminalId);
  assert.equal(await username.inputValue({ timeout: 1000 }), "未保存资料");
  assert.deepEqual(errors, []);
  console.log("PASS: notification test requests permission and notification clicks preserve Settings drafts");
} finally {
  await browser.close();
}
