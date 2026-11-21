import assert from "node:assert/strict";
import { createWorkbench, chooseInline } from "./workbench-fixture.mjs";

for (const mode of ["subscription", "yaml", "saved", "add-to-saved"]) {
  let imports = 0, applies = 0;
  const addingToSaved = mode === "add-to-saved";
  const hasSaved = mode === "saved" || addingToSaved;
  const sources = hasSaved ? [{
    id: "saved", name: "已有来源", nodes: [
      { id: "saved-node", name: "已有节点", type: "http" },
      { id: "draft-node", name: "已有节点备用", type: "http" },
    ],
  }] : [];
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
        assert.equal(value.name, "新来源");
        if (mode === "subscription" || addingToSaved) {
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
    const current = page.getByRole("list", { name: "代理配置步骤", exact: true }).locator('[aria-current="step"]');
    await current.waitFor({ timeout: 5000 });
    const next = page.getByRole("button", { name: "下一步：确认应用", exact: true });
    if (addingToSaved) {
      await page.getByRole("button", { name: "选择节点 已有节点备用", exact: true }).click();
      await chooseInline(page.getByRole("combobox", { name: "节点来源", exact: true }), "saved");
      await page.getByLabel("搜索节点", { exact: true }).fill("备用");
      await page.getByRole("button", { name: "返回导入", exact: true }).click();
    }
    if (mode !== "saved") {
      assert.match(await current.innerText(), /导入来源/);
      await page.getByLabel("来源名称", { exact: true }).fill("新来源");
      if (mode === "subscription" || addingToSaved) {
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
    await page.getByRole("heading", { name: "选择节点", exact: true }).waitFor();
    assert.match(await current.innerText(), /选择节点/);
    if (addingToSaved) {
      assert.equal(await page.getByLabel("搜索节点", { exact: true }).inputValue(), "备用", "import cleared node search");
      assert.match(await page.getByRole("combobox", { name: "节点来源", exact: true }).innerText(), /已有来源/, "import replaced the source filter");
    }
    const chosenName = addingToSaved ? "已有节点备用" : mode === "saved" ? "已有节点" : "新节点";
    const choose = page.getByRole("button", { name: `选择节点 ${chosenName}`, exact: true });
    await choose.waitFor();
    assert.equal(applies, 0);
    if (!hasSaved) assert.equal(await next.isDisabled(), true);
    if (addingToSaved) assert.equal(await choose.getAttribute("aria-pressed"), "true", "import cleared the unapplied selection");
    await choose.click();
    await next.click();
    assert.match(await current.innerText(), /应用代理/);
    assert.equal(applies, 0);
    if (addingToSaved) {
      const review = page.getByRole("region", { name: "确认待应用代理", exact: true });
      await review.getByRole("alert").filter({ hasText: "原代理链路尚未就绪" }).waitFor();
      await review.getByRole("button", { name: "撤销修改", exact: true }).waitFor();
    }
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
