import assert from "node:assert/strict";
import { createWorkbench } from "./workbench-fixture.mjs";

for (const mode of ["subscription", "yaml", "saved"]) {
  let imports = 0, applies = 0;
  const sources = mode === "saved" ? [{ id: "saved", name: "已有来源", nodes: [{ id: "saved-node", name: "已有节点", type: "http" }] }] : [];
  let selected = mode === "saved" ? "saved-node" : "";
  const { browser, page, errors } = await createWorkbench({
    waitForReady: false,
    beforeNavigate: async (page, state) => {
      await page.route("**/api/desktop-license", route => route.fulfill({ json: { state: "active" } }));
      await page.route("**/api/proxy", route => route.fulfill({ json: {
        active: Boolean(selected), running: Boolean(selected), selected, sources, port: 17896,
        core_available: true, network_open: false, state_label: selected ? "待启动环境" : "未配置",
      } }));
      await page.route("**/api/proxy/import", route => {
        imports++;
        const value = route.request().postDataJSON();
        assert.equal(value.name, "新来源");
        if (mode === "subscription") {
          assert.equal(value.url, "https://proxy.example.invalid/config");
          assert.equal(value.host_download, true);
        } else {
          assert.match(value.content, /new-node/);
          assert.equal(value.url, "");
        }
        sources.push({ id: "new", name: "新来源", nodes: [{ id: "new-node", name: "新节点", type: "http" }] });
        state.status.job = { id: "source-import", kind: "proxy", running: false };
        return route.fulfill({ json: state.status.job });
      });
      await page.route("**/api/proxy/route", route => {
        applies++;
        assert.deepEqual(route.request().postDataJSON().node_ids, [mode === "saved" ? "saved-node" : "new-node"]);
        selected = mode === "saved" ? "saved-node" : "new-node";
        state.status.job = { id: "route-apply", kind: "proxy", running: false, message: "代理已应用" };
        return route.fulfill({ json: state.status.job });
      });
    },
  });
  try {
    await page.getByRole("button", { name: /^设置/ }).click();
    await page.getByRole("button", { name: "网络与代理", exact: true }).click();
    const current = page.getByRole("list", { name: "代理配置步骤", exact: true }).locator('[aria-current="step"]');
    await current.waitFor({ timeout: 5000 });
    if (mode !== "saved") {
      assert.match(await current.innerText(), /导入来源/);
      await page.getByLabel("来源名称", { exact: true }).fill("新来源");
      if (mode === "subscription") {
        await page.getByLabel("订阅链接", { exact: true }).fill("https://proxy.example.invalid/config");
        await page.getByLabel("通过主机网络下载这份订阅").check();
      } else {
        await page.getByRole("button", { name: "本地 YAML", exact: true }).click();
        await page.locator('input[type="file"]').setInputFiles({
          name: "proxy.yaml", mimeType: "text/yaml",
          buffer: Buffer.from("proxies: [{name: new-node, type: http, server: 127.0.0.1, port: 8080}]"),
        });
        await page.setViewportSize({ width: 600, height: 740 });
      }
      await page.getByRole("button", { name: "导入并验证", exact: true }).click();
    }
    const choose = page.getByRole("button", { name: mode === "saved" ? "选择节点 已有节点" : "选择节点 新节点" });
    await choose.waitFor();
    assert.match(await current.innerText(), /选择节点/);
    assert.equal(applies, 0);
    const next = page.getByRole("button", { name: "下一步：确认应用", exact: true });
    if (mode !== "saved") assert.equal(await next.isDisabled(), true);
    await choose.click();
    await next.click();
    assert.match(await current.innerText(), /应用代理/);
    assert.equal(applies, 0);
    await page.getByRole("button", { name: "返回选择节点", exact: true }).click();
    assert.equal(await choose.getAttribute("aria-pressed"), "true");
    await next.click();
    await page.getByRole("button", { name: mode === "saved" ? "重新应用" : "应用代理", exact: true }).click();
    await page.getByText("代理已应用", { exact: true }).waitFor();
    assert.equal(imports, mode === "saved" ? 0 : 1);
    assert.equal(applies, 1);
    assert.deepEqual(errors, []);
    console.log(`PASS ${mode}: import/skip, selection, return, explicit apply`);
  } finally {
    await browser.close();
  }
}
