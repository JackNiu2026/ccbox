// Decisions for the virtualization notice shown before a Linux guest install.
// The server diagnoses the host; this module only maps its report to actions.

const linuxGuests = new Set(["debian-12", "debian-13", "ubuntu-22.04", "ubuntu-24.04"]);

export function isLinuxGuest(system) {
  return linuxGuests.has(system);
}

// Actions in display order. Only the ones the report offers are returned.
export function virtualizationActions(report) {
  if (!report || report.ready) return [];
  const actions = [];
  if (report.can_enable) actions.push({ action: "enable", label: "一键启用", primary: true });
  if (report.needs_restart) actions.push({ action: "restart", label: "立即重启", primary: !report.can_enable });
  if (report.firmware_restart) actions.push({ action: "firmware", label: "重启并进入 BIOS 设置" });
  return actions;
}

export const actionProgress = {
  enable: "正在等待 Windows 管理员授权并启用，可能需要几分钟…",
  restart: "正在发起重启…",
  firmware: "正在等待 Windows 管理员授权…",
};

// Restarts end every running program, so they always ask first.
export const actionConfirmation = {
  restart: "电脑将在 15 秒后重启。请先保存其他程序中的工作。确定现在重启吗？",
  firmware: "电脑将在 15 秒后重启并进入 BIOS/UEFI 设置。请先保存其他程序中的工作，并记下页面上的设置步骤。确定现在重启吗？",
};

export const actionDone = {
  restart: "电脑将在 15 秒内重启。重启后回到 CCBox 继续安装。",
  firmware: "电脑将在 15 秒内重启并进入 BIOS/UEFI 设置。按页面上的步骤开启虚拟化，保存后回到 CCBox 继续安装。",
};

// The install button stays available until a check says the host is blocked;
// the server applies the same check before downloading anything.
export function hostAllowsInstall(choice) {
  return !(choice && isLinuxGuest(choice.guest_system) && choice.host_ready === false);
}
