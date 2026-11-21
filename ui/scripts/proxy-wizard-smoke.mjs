import assert from "node:assert/strict";
import { createWorkbench, chooseInline } from "./workbench-fixture.mjs";

for (const mode of ["subscription", "yaml", "pasted-yaml", "manual", "saved", "add-to-saved"]) {
  let imports = 0, applies = 0;
  const addingToSaved = mode === "add-to-saved";
  const hasSaved = mode === "saved" || addingToSaved;
  const sources = hasSaved ? [{
    id: "saved", name: "已有来源", nodes: [
      { id: "saved-node", name: "已有节点", type: "http" },
      { id: "draft-node", name: "已有节点备用", type: "http" },
    ],
  }] : [];
  const usesSubscription = mode === "subscription" || addingToSaved;
  const sourceName = usesSubscription ? "proxy.example.invalid" : mode === "yaml" ? "proxy" : mode === "manual" ? "新节点" : "本地 YAML";
  const expectedNode = addingToSaved ? "draft-node" : mode === "saved" ? "saved-node" : "new-node";
  let selected = hasSaved ? "saved-node" : "";
  const { browser, page, errors } = await createWorkbench({
    waitForReady: false,
    beforeNavigate: async (page, state) => {
      await page.route("**/api/desktop-license", route => route.fulfill({ json: { state: "active" } }));
      await page.route("**/api/proxy", route => route.fulfill({ json: {
        active: Boolean(selected), running: Boolean(selected), selected, sources, port: 17896,
        core_available: true, network_open: false, state_label: selected ? "待启动环境" : "未配置",
        route_error: addingToSaved ? "原代理链路尚未就绪" : "",
      } }));
      await page.route("**/api/proxy/import", route => {
        imports++;
        const value = route.request().postDataJSON();
        assert.equal(value.name, sourceName);
        assert.equal(value.host_download, mode === "subscription");
        if (usesSubscription) {
          assert.equal(value.url, "https://proxy.example.invalid/config?token=fixture-secret");
        } else {
          assert.match(value.content, mode === "manual" ? /新节点/ : /new-node/);
          assert.equal(value.url, "");
        }
        sources.push({ id: "new", name: sourceName, nodes: [{ id: "new-node", name: "新节点", type: "http" }] });
        state.status.job = { id: "source-import", kind: "proxy", running: false };
        return route.fulfill({ json: state.status.job });
      });
      await page.route("**/api/proxy/route", route => {
        applies++;
        assert.deepEqual(route.request().postDataJSON(), { node_ids: [expectedNode], port: 17896 });
        selected = expectedNode;
        state.status.job = { id: "route-apply", kind: "proxy", running: false, message: "代理已应用" };
        return route.fulfill({ json: state.status.job });
      });
    },
  });
  try {
    await page.getByRole("button", { name: /^设置/ }).click();
    await page.getByRole("button", { name: "网络与代理", exact: true }).click();
    const tab = page.getByRole("tab", { name: "节点", exact: true });
    await tab.waitFor();
    assert.equal(await tab.getAttribute("aria-selected"), "true");
    const importSection = page.getByRole("region", { name: "导入来源", exact: true });
    const nodeSection = page.getByRole("region", { name: "选择节点", exact: true });
    const apply = page.getByRole("button", { name: mode === "saved" ? "重新应用" : "应用代理", exact: true });
    if (addingToSaved) {
      await page.getByRole("button", { name: "选择节点 已有节点备用", exact: true }).click();
      await chooseInline(page.getByRole("combobox", { name: "节点来源", exact: true }), "saved");
      await page.getByLabel("搜索节点", { exact: true }).fill("备用");
      await page.getByRole("button", { name: "添加来源", exact: true }).click();
      assert.ok(await nodeSection.isVisible(), "opening import must leave the node section visible");
      assert.equal(await nodeSection.getByLabel("搜索节点", { exact: true }).inputValue(), "备用");
    }
    if (mode !== "saved") {
      await importSection.waitFor({ timeout: 5000 });
      assert.equal(await tab.getAttribute("aria-selected"), "true", "import must stay on the node tab");
      if (!addingToSaved) assert.equal(await nodeSection.count(), 0);
      assert.equal(await page.getByLabel("来源名称", { exact: true }).count(), 0, "new sources must not require a display name");
      assert.equal(await page.getByLabel("通过主机网络下载这份订阅").count(), 0, "download route must be automatic");
      if (usesSubscription) {
        await page.getByLabel("订阅链接", { exact: true }).fill("https://proxy.example.invalid/config?token=fixture-secret");
      } else if (mode === "manual") {
        await page.getByRole("button", { name: "手动节点", exact: true }).click();
        await page.getByLabel("节点名称", { exact: true }).fill("新节点");
        await page.getByLabel("服务器", { exact: true }).fill("127.0.0.1");
      } else {
        await page.getByRole("button", { name: "本地 YAML", exact: true }).click();
        const yaml = "proxies: [{name: new-node, type: http, server: 127.0.0.1, port: 8080}]";
        if (mode === "pasted-yaml") {
          await page.getByRole("textbox", { name: "节点 YAML", exact: true }).fill(yaml);
        } else {
          await page.locator('input[type="file"]').setInputFiles({
            name: "proxy.yaml", mimeType: "text/yaml", buffer: Buffer.from(yaml),
          });
          await page.setViewportSize({ width: 600, height: 740 });
        }
      }
      await page.getByRole("button", { name: "导入并验证", exact: true }).click();
    }
    await nodeSection.waitFor();
    assert.equal(await tab.getAttribute("aria-selected"), "true");
    await importSection.waitFor({ state: "hidden" });
    assert.equal(await page.getByRole("button", { name: "返回导入", exact: true }).count(), 0);
    if (addingToSaved) {
      assert.equal(await page.getByLabel("搜索节点", { exact: true }).inputValue(), "备用", "import cleared node search");
      assert.match(await page.getByRole("combobox", { name: "节点来源", exact: true }).innerText(), /已有来源/, "import replaced the source filter");
    }
    const chosenName = addingToSaved ? "已有节点备用" : mode === "saved" ? "已有节点" : "新节点";
    const choose = page.getByRole("button", { name: `选择节点 ${chosenName}`, exact: true });
    await choose.waitFor();
    assert.equal(applies, 0);
    if (!hasSaved) assert.equal(await apply.isDisabled(), true);
    if (addingToSaved) {
      assert.equal(await choose.getAttribute("aria-pressed"), "true", "import cleared the unapplied selection");
      await page.getByRole("alert").filter({ hasText: "原代理链路尚未就绪" }).waitFor();
      await page.getByRole("button", { name: "撤销修改", exact: true }).waitFor();
    }
    if (mode !== "saved") await choose.click();
    assert.equal(await page.getByRole("button", { name: "下一步：确认应用", exact: true }).count(), 0);
    assert.equal(applies, 0, "selecting a candidate must leave the existing route active until Apply");
    await apply.click();
    await page.getByText("代理已应用", { exact: true }).waitFor();
    assert.equal(imports, mode === "saved" ? 0 : 1);
    assert.equal(applies, 1);
    assert.deepEqual(errors, []);
    console.log(`PASS ${mode}: same-page import, automatic defaults and one-click apply`);
  } finally {
    await browser.close();
  }
}
