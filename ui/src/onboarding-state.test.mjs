import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { isHostNetworkDetectionFailure, needsProxySetup, setupFailureMessage } from "./onboarding-state.mjs";

test("only an applied CCBox node completes setup", () => {
  assert.equal(needsProxySetup({}, {}, {}), true);
  assert.equal(
    needsProxySetup({}, {}, { active: false, selected: "node" }),
    true,
  );
  assert.equal(
    needsProxySetup({}, {}, { active: true, selected: "node" }),
    false,
  );
});
test("running VM, old endpoint and restore file cannot bypass node setup", () => {
  for (const status of [{ vm_running: true }, { can_restore: true }]) {
    assert.equal(
      needsProxySetup({ proxy_url: "socks5://127.0.0.1:17896" }, status, {}),
      true,
    );
  }
});

test("host adapter failures use their own recovery guidance", () => {
  for (const message of [
    "启动环境失败：无法读取 Windows 物理网卡信息，请确认系统网络服务正常后重试",
    "未找到可用的物理 IPv4 网卡",
    "Windows 物理网卡查询超时",
    "无法读取 Windows IPv4 路由",
    "Windows 物理网卡没有可用的 IPv4 默认路由",
  ]) assert.equal(isHostNetworkDetectionFailure(message), true);
  for (const message of [undefined, "", "准备 Debian 13 镜像失败：下载超时", "代理 TLS 握手失败"]) {
    assert.equal(isHostNetworkDetectionFailure(message), false);
  }
});

test("setup boot errors match localized diagnostics while retaining unfamiliar backend errors", () => {
  const reason = "虚拟机后台在环境就绪前退出";
  const help = "请打开“操作详情”查看启动记录。若继续安装仍失败，可从安装页底部的“日志与诊断”导出诊断包。";
  const original = `启动 Linux 基础环境失败：${reason}。${help}`;
  const translations = {
    "启动 Linux 基础环境失败：{0}。{1}": "Boot failed: {0}. {1}",
    [reason]: "the VM process exited early",
    [help]: "Open operation details.",
    "启动 Linux 基础环境失败：虚拟机未能启动。QEMU 启动失败：{0}。请查看操作详情；仍无法启动时，在设置中导出诊断包。": "The virtual machine could not start: {0}.",
  };
  const translate = (key, ...values) => (translations[key] || key).replace(/\{(\d+)\}/g, (_, index) => values[index]);
  assert.equal(setupFailureMessage(original, translate), "Boot failed: the VM process exited early. Open operation details.");
  const qemu = "启动 Linux 基础环境失败：虚拟机未能启动。QEMU 启动失败：WHPX: unavailable。请查看操作详情；仍无法启动时，在设置中导出诊断包。";
  assert.equal(setupFailureMessage(qemu, translate), "The virtual machine could not start: WHPX: unavailable.");
  const unfamiliar = "启动 Linux 基础环境失败：其他原因。";
  assert.equal(setupFailureMessage(unfamiliar, translate), unfamiliar);
});

test("all shipped locales translate the first-setup boot failure and resume controls", () => {
  const languages = JSON.parse(readFileSync(new URL("./locales/languages.json", import.meta.url)));
  assert.deepEqual(languages.map(({ id }) => id), ["zh-CN", "zh-TW", "en"]);
  const keys = [
    "继续完成环境安装", "环境安装进度", "系统启动", "Linux 基础环境启动失败", "正在启动 Linux 基础环境",
    "可在原安装目录继续，无需删除已下载的镜像。", "继续安装", "查看操作详情", "初始化操作详情",
    "启动 Linux 基础环境失败：{0}。{1}", "Linux 客机未报告初始化完成", "虚拟机后台在环境就绪前退出",
    "无法读取虚拟机后台的启动状态", "等待虚拟机就绪超时",
  ];
  for (const { id } of languages.filter(({ id }) => id !== "zh-CN")) {
    const catalog = JSON.parse(readFileSync(new URL(`./locales/${id}.json`, import.meta.url)));
    for (const key of keys) {
      assert.ok(catalog[key]?.trim(), `${id}: ${key}`);
      assert.deepEqual(catalog[key].match(/\{\d+\}/g)?.sort(), key.match(/\{\d+\}/g)?.sort(), `${id}: ${key}`);
    }
  }
});

test("English and Traditional Chinese cover the same desktop message keys", () => {
  const catalog = id => JSON.parse(readFileSync(new URL(`./locales/${id}.json`, import.meta.url)));
  const english = catalog("en");
  const traditional = catalog("zh-TW");
  assert.deepEqual(Object.keys(english).sort(), Object.keys(traditional).sort());
  for (const [message, translation] of Object.entries(english)) {
    assert.doesNotMatch(translation, /\p{Script=Han}/u, message);
    assert.deepEqual(translation.match(/\{\d+\}/g)?.sort(), message.match(/\{\d+\}/g)?.sort(), message);
  }
});
