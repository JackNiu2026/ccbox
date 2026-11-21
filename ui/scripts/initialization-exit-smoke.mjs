import assert from "node:assert/strict";
import { createWorkbench } from "./workbench-fixture.mjs";

let starts = 0;
const { browser, page, errors } = await createWorkbench({
  waitForReady: false,
  initialization: { required: true, tracked: true, started: true, prepared: true },
  clientContext: { mode: "local", connection_id: "local", client_platform: "windows", server_platform: "windows" },
  beforeNavigate: async (page, state) => {
    state.status.local_vm_storage = true;
    await page.route("**/api/desktop-license", route => route.fulfill({ json: { state: "active" } }));
    state.status.job = {
      id: "rejected-exit", kind: "start", running: false, message: "启动环境失败",
      error: "检测出口命中中国大陆 IP 网段，Cage 禁止使用大陆出口，外网保持关闭",
    };
    await page.route("**/api/environment/guest-systems", route => route.fulfill({ json: { pending: { system: "ubuntu-22.04" } } }));
    await page.route("**/api/start", route => {
      starts++;
      assert.equal(route.request().postDataJSON().guest_system, "ubuntu-22.04");
      state.status.job = { id: "resumed-install", kind: "start", running: true, message: "正在继续原安装" };
      return route.fulfill({ json: state.status.job });
    });
  },
});
try {
  await page.getByRole("heading", { name: "继续完成环境安装", exact: true }).waitFor({ timeout: 7000 });
  assert.equal(await page.getByRole("button", { name: "继续安装", exact: true }).count(), 0,
    "A rejected exit must not offer to restart installation with the same proxy.");
  const recovery = page.getByRole("alert").filter({ hasText: "CCBox 外网保持关闭" });
  assert.equal(await page.getByText(/Cage 禁止使用大陆出口/).count(), 0);
  await recovery.getByRole("button", { name: "更换 CCBox 代理节点", exact: true }).click();
  assert.equal(starts, 0);
  await page.getByRole("heading", { name: "连接你的网络代理", exact: true }).waitFor();
  await page.getByRole("button", { name: "开始安装", exact: true }).click();
  await page.getByText("正在继续原安装", { exact: true }).waitFor();
  assert.equal(starts, 1, "The pending installation must resume from the proxy step.");
  assert.deepEqual(errors, []);
  console.log("PASS: Windows setup recovers a blocked proxy exit without restarting the pending installation.");
} catch (error) {
  console.error("Page:", (await page.locator("body").innerText()).slice(0, 1600), errors);
  throw error;
} finally {
  await browser.close();
}
