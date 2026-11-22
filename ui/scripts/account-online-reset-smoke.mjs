import assert from "node:assert/strict";
import { createWorkbench } from "./workbench-fixture.mjs";

for (const existing of [false, true]) {
  let resets = 0;
  const { browser, page, errors } = await createWorkbench({
    waitForReady: false,
    beforeNavigate: async (page, state) => {
      Object.assign(state.account, {
        exists: existing, unlocked: false, online_enabled: true,
        online_username: existing ? "fixture_account" : "",
      });
      await page.route("**/api/account/online-reset", route => {
        resets++;
        assert.deepEqual(route.request().postDataJSON(), {
          username: "fixture_account", reset_code: "1111-2222-3333",
          password: "new-fixture-password", confirm_password: "new-fixture-password",
        });
        return route.fulfill({ json: state.account });
      });
    },
  });
  try {
    if (!existing) await page.getByRole("button", { name: "已有 CCBox 账户？登录", exact: true }).click();
    await page.getByRole("button", { name: "忘记密码？", exact: true }).click();
    const reset = page.getByRole("region", { name: "重置线上账户密码", exact: true });
    await reset.waitFor();
    assert.equal(await page.getByText(/恢复密钥/).count(), 0);
    if (existing) {
      assert.equal(await reset.getByLabel("用户名", { exact: true }).inputValue(), "fixture_account");
      await reset.getByText(/本机加密资料仍需使用原密码解锁/).waitFor();
    } else await reset.getByLabel("用户名", { exact: true }).fill("fixture_account");
    await reset.getByLabel("重置码", { exact: true }).fill("1111-2222-3333");
    await reset.getByLabel("新密码", { exact: true }).fill("new-fixture-password");
    await reset.getByLabel("确认密码", { exact: true }).fill("incorrect-fixture-password");
    const submit = reset.getByRole("button", { name: "重置密码", exact: true });
    await submit.click();
    await reset.getByRole("alert").getByText("两次密码不一致").waitFor();
    assert.equal(resets, 0);
    await reset.getByLabel("确认密码", { exact: true }).fill("new-fixture-password");
    await submit.click();
    await page.getByRole("status").filter({ hasText: "线上密码已重置" }).waitFor();
    await page.getByRole("button", { name: existing ? "解锁 CCBox" : "登录", exact: true }).waitFor();
    assert.equal(resets, 1);
    assert.deepEqual(errors, []);
    console.log(`PASS ${existing ? "existing device" : "first sign-in"}: online reset and return to sign-in`);
  } finally { await browser.close(); }
}

const { browser, page, errors } = await createWorkbench();
try {
  await page.getByRole("button", { name: /^账户菜单，/ }).click();
  await page.getByRole("menuitem", { name: "设置", exact: true }).click();
  await page.getByRole("heading", { name: "账户密码", exact: true }).waitFor();
  assert.equal(await page.getByText(/恢复密钥/).count(), 0);
  assert.deepEqual(errors, []);
  console.log("PASS account settings: recovery-key controls removed");
} finally { await browser.close(); }
