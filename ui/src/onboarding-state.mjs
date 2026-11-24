// Starting always requires a node applied by the CCBox proxy manager.
export function needsProxySetup(_settings, _status, proxy) {
  return !(proxy?.active && proxy?.selected);
}

export function isHostNetworkDetectionFailure(message) {
  return typeof message === "string" &&
    /物理(?: IPv4)?\s*网卡|Windows IPv4 路由/.test(message);
}

const bootFailurePrefix = "启动 Linux 基础环境失败：";
const qemuFailurePrefix = bootFailurePrefix + "虚拟机未能启动。QEMU 启动失败：";
const qemuFailureSuffix = "。请查看操作详情；仍无法启动时，在设置中导出诊断包。";
const bootFailureReasons = [
  "Linux 客机未报告初始化完成",
  "虚拟机后台在环境就绪前退出",
  "无法读取虚拟机后台的启动状态",
  "等待虚拟机就绪超时",
];

export function setupFailureMessage(message, translate) {
  if (!message?.startsWith(bootFailurePrefix)) return message;
  if (message.startsWith(qemuFailurePrefix) && message.endsWith(qemuFailureSuffix)) {
    return translate(qemuFailurePrefix + "{0}" + qemuFailureSuffix,
      message.slice(qemuFailurePrefix.length, -qemuFailureSuffix.length));
  }
  const reason = bootFailureReasons.find(text => message.startsWith(bootFailurePrefix + text + "。"));
  if (!reason) return message;
  const help = message.slice(bootFailurePrefix.length + reason.length + 1);
  return translate(bootFailurePrefix + "{0}。{1}", translate(reason), translate(help));
}
