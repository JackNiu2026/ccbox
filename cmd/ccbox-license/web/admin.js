// CCBox license administration. Dependency-free and served under a strict
// CSP: every value from the server is inserted as text, never as HTML.

const API = "api/";
const app = document.getElementById("app");
const toasts = document.getElementById("toasts");
let session = null;
let config = { duration_days: 30, max_batch: 500, max_note: 80 };

// ---------------------------------------------------------------- DOM helpers

function h(tag, attrs, ...children) {
  const element = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === "class") element.className = value;
    else if (key === "dataset") Object.assign(element.dataset, value);
    else if (key.startsWith("on")) element.addEventListener(key.slice(2), value);
    else if (key === "style") element.style.cssText = value;
    else if (value === true) element.setAttribute(key, "");
    else element.setAttribute(key, String(value));
  }
  append(element, children);
  return element;
}

function append(parent, children) {
  for (const child of children.flat(Infinity)) {
    if (child === undefined || child === null || child === false) continue;
    parent.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return parent;
}

const SVG = "http://www.w3.org/2000/svg";
const ICONS = {
  overview: ["M3 13h8V3H3z", "M13 21h8V11h-8z", "M3 21h8v-6H3z", "M13 3v6h8V3z"],
  codes: ["M15 7a2 2 0 0 1 2 2", "M21 9a6 6 0 0 1-7.7 5.7L11 17H9v2H7v2H3v-3.6l5.3-5.3A6 6 0 1 1 21 9z"],
  batches: ["M21 8 12 3 3 8l9 5 9-5z", "m3 13 9 5 9-5", "m3 17.5 9 5 9-5"],
  devices: ["M4 5h16v11H4z", "M8 20h8", "M12 16v4"],
  users: ["M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2", "M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z", "M22 21v-2a4 4 0 0 0-3-3.9", "M16 3.1a4 4 0 0 1 0 7.8"],
  audit: ["M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2", "M9 3h6v4H9z", "M9 12h6", "M9 16h4"],
  plus: ["M12 5v14", "M5 12h14"],
  search: ["M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14z", "m21 21-4.3-4.3"],
  copy: ["M9 9h11v11H9z", "M5 15H4V4h11v1"],
  download: ["M12 3v12", "m7 10 5 5 5-5", "M5 21h14"],
  refresh: ["M21 12a9 9 0 1 1-2.6-6.4", "M21 3v6h-6"],
  transfer: ["M7 7h13", "m16 3 4 4-4 4", "M17 17H4", "m8 21-4-4 4-4"],
  renew: ["M12 8v4l3 2", "M21 12a9 9 0 1 1-9-9", "M21 3v5h-5"],
  ban: ["M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z", "m5.7 5.7 12.6 12.6"],
  warn: ["M12 9v4", "M12 17h.01", "M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"],
  info: ["M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z", "M12 16v-4", "M12 8h.01"],
  inbox: ["M22 12h-6l-2 3h-4l-2-3H2", "M5.5 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.7 4H7.3a2 2 0 0 0-1.8 1.1z"],
  close: ["M18 6 6 18", "m6 6 12 12"],
  lock: ["M5 11h14v10H5z", "M8 11V7a4 4 0 0 1 8 0v4"],
  eye: ["M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z", "M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z"],
};

function icon(name) {
  const svg = document.createElementNS(SVG, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  svg.setAttribute("aria-hidden", "true");
  for (const d of ICONS[name] || []) {
    const path = document.createElementNS(SVG, "path");
    path.setAttribute("d", d);
    svg.append(path);
  }
  return svg;
}

function toast(message, kind = "ok") {
  const element = h("div", { class: `toast ${kind}` }, message);
  toasts.append(element);
  setTimeout(() => element.remove(), kind === "error" ? 6000 : 3200);
}

// ------------------------------------------------------------------ formatting

const dateTime = new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" });
const dateOnly = new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium" });
const fmtTime = seconds => (seconds ? dateTime.format(new Date(seconds * 1000)) : "—");
const shortFormats = {
  current: new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }),
  other: new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }),
};
const fmtShort = seconds => {
  if (!seconds) return "—";
  const date = new Date(seconds * 1000);
  return shortFormats[date.getFullYear() === new Date().getFullYear() ? "current" : "other"].format(date);
};
const fmtDate = seconds => (seconds ? dateOnly.format(new Date(seconds * 1000)) : "—");
const fmtNumber = value => (typeof value === "string" ? value : new Intl.NumberFormat("zh-CN").format(value || 0));
const shortDevice = device => (device ? `${device.slice(0, 8)}…${device.slice(-4)}` : "—");
const nowSeconds = () => Math.floor(Date.now() / 1000);

function remaining(expires) {
  const left = expires - nowSeconds();
  if (left <= 0) return `已过期 ${Math.max(1, Math.floor(-left / 86400))} 天`;
  if (left < 86400) return `剩余 ${Math.max(1, Math.floor(left / 3600))} 小时`;
  return `剩余 ${Math.floor(left / 86400)} 天`;
}

const CODE_STATUS = {
  unused: ["未使用", "info"],
  redeemed: ["已兑换", "ok"],
  pending: ["待兑换", "warn"],
  revoked: ["已停用", "bad"],
};
const CODE_KIND = { code: "激活码", transfer: "换机码", renewal: "续期码" };
const DEVICE_STATUS = {
  active: ["有效", "ok"],
  expired: ["已到期", "bad"],
  transferred: ["已转出", ""],
};
const AUDIT_ACTION = {
  login: "登录",
  "login.failed": "登录失败",
  logout: "退出登录",
  "codes.issue": "生成激活码",
  "codes.revoke": "停用激活码",
  "batches.revoke": "停用批次",
  "devices.transfer": "换机",
  "devices.renew": "设备续期",
  "users.disable": "停用用户",
  "users.enable": "恢复用户",
  "users.adjust": "调整用户期限",
  "users.device-remove": "解绑用户设备",
  "users.reset": "发放重置码",
};
const USER_STATUS = {
  active: ["有效", "ok"],
  expired: ["已到期", "bad"],
  none: ["未授权", "info"],
  disabled: ["已停用", "bad"],
};
const EVENT_SOURCE = { code: "兑换激活码", legacy: "旧版设备授权转入", admin: "后台调整" };

const badge = ([label, kind]) => h("span", { class: `badge ${kind}` }, label);

// ------------------------------------------------------------------------- API

class HttpError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

async function api(path, body) {
  const options = { credentials: "same-origin", headers: { Accept: "application/json" } };
  if (body !== undefined) {
    options.method = "POST";
    options.headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  let response;
  try {
    response = await fetch(API + path, options);
  } catch {
    throw new HttpError(0, "无法连接授权服务，请检查网络");
  }
  let data = null;
  try {
    data = await response.json();
  } catch {
    /* non-JSON error page */
  }
  if (!response.ok) {
    const error = new HttpError(response.status, data?.error || `请求失败（${response.status}）`);
    if (response.status === 401 && path !== "login") {
      session = null;
      renderLogin("登录已过期，请重新登录。");
    }
    throw error;
  }
  return data;
}

function query(params) {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) if (value !== "" && value !== undefined && value !== null) search.set(key, value);
  const text = search.toString();
  return text ? `?${text}` : "";
}

async function copyText(text, label = "已复制") {
  try {
    await navigator.clipboard.writeText(text);
    toast(label);
  } catch {
    toast("浏览器不允许自动复制，请手动选择复制", "error");
  }
}

function download(name, text, type = "text/plain") {
  const url = URL.createObjectURL(new Blob(["﻿" + text], { type: `${type};charset=utf-8` }));
  const link = h("a", { href: url, download: name });
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

const csvCell = value => {
  const text = String(value ?? "");
  const safe = /^[=+\-@\t\r]/.test(text) ? `'${text}` : text;
  return /[",\n]/.test(safe) ? `"${safe.replaceAll('"', '""')}"` : safe;
};
const toCSV = rows => rows.map(row => row.map(csvCell).join(",")).join("\n") + "\n";
const stamp = () => new Date().toISOString().slice(0, 16).replace(/[-:T]/g, "");

// ---------------------------------------------------------------------- Login

const LOGIN_RULES = [
  ["SESSION", "闲置 45 分钟或登录 8 小时后失效"],
  ["LIMIT", "同一来源 15 分钟内失败 8 次暂停登录"],
  ["ORIGIN", "所有写操作校验浏览器来源"],
  ["AUDIT", "登录与每次写操作写入操作日志"],
];

function renderLogin(message = "") {
  closeDialogs();
  const error = h("p", { class: "form-error", id: "password-error", role: "alert" }, message);
  const account = h("input", { id: "username", type: "text", autocomplete: "username", autocapitalize: "none", spellcheck: "false", required: true, maxlength: 64, "aria-describedby": "password-error" });
  const input = h("input", { id: "password", type: "password", autocomplete: "current-password", required: true, minlength: 1, "aria-describedby": "password-error" });
  const reveal = h("button", { class: "reveal", type: "button", "aria-label": "显示密码", "aria-pressed": "false", onclick: () => {
    const shown = input.type === "password";
    input.type = shown ? "text" : "password";
    reveal.setAttribute("aria-label", shown ? "隐藏密码" : "显示密码");
    reveal.setAttribute("aria-pressed", String(shown));
    input.focus();
  } }, icon("eye"));
  const field = h("div", { class: "password-field" }, input, reveal);
  const button = h("button", { class: "btn primary", type: "submit" }, "登录");
  const accountField = h("div", { class: "password-field" }, account);
  const fail = (text, target = input) => {
    error.textContent = text;
    const box = target === account ? accountField : field;
    box.classList.add("invalid");
    target.setAttribute("aria-invalid", "true");
    target.select();
  };
  const clear = () => {
    error.textContent = "";
    for (const box of [accountField, field]) box.classList.remove("invalid");
    for (const target of [account, input]) target.removeAttribute("aria-invalid");
  };
  account.addEventListener("input", clear);
  input.addEventListener("input", clear);
  const form = h("form", { class: "login-form", "aria-labelledby": "login-title", novalidate: true, onsubmit: async event => {
    event.preventDefault();
    if (!account.value.trim()) return fail("请输入管理员账号", account);
    if (!input.value) return fail("请输入管理员密码");
    button.disabled = true;
    button.textContent = "正在验证…";
    error.textContent = "";
    try {
      await api("login", { username: account.value.trim(), password: input.value });
      input.value = "";
      await boot();
    } catch (failure) {
      fail(failure.message);
    } finally {
      button.disabled = false;
      button.textContent = "登录";
    }
  } },
    h("div", {}, h("h2", { id: "login-title" }, "登录"), h("p", { class: "login-sub" }, "使用服务器配置的管理员账号和密码。")),
    h("div", { class: "login-fields" },
      h("div", { class: "field" }, h("label", { for: "username" }, "账号"), accountField),
      h("div", { class: "field" }, h("label", { for: "password" }, "密码"), field, error)),
    button,
    h("p", { class: "login-foot" }, "管理路由仅在设置 ", h("code", {}, "CCBOX_LICENSE_ADMIN_PASSWORD_FILE"),
      " 后开启。建议在代理入口限制管理员来源或使用管理 VPN。"),
  );
  const brand = h("section", { class: "login-brand", "aria-label": "CCBox 授权服务" },
    h("div", { class: "login-mark" }, h("img", { src: "assets/icon.svg", alt: "" }), h("strong", {}, "CCBox"), h("span", { class: "login-tag" }, "LICENSE CONSOLE")),
    h("div", { class: "login-pitch" },
      h("h1", {}, "授权管理", h("em", {}, "后台")),
      h("p", {}, "发放激活码、查询兑换、为设备续期和换机。服务端只保存激活码哈希，签名私钥不离开服务器。"),
      h("ul", { class: "login-rules" }, LOGIN_RULES.map(([tag, text]) => h("li", {}, h("span", {}, tag), text)))),
    h("p", { class: "login-path" }, "ccbox-license · /admin/license/"));
  app.replaceChildren(h("main", { class: "login" }, brand, h("div", { class: "login-side" }, form)));
  account.focus();
}

// ---------------------------------------------------------------------- Shell

const ROUTES = [
  { id: "overview", label: "概览", icon: "overview", render: renderOverview },
  { id: "users", label: "用户", icon: "users", render: renderUsers },
  { id: "codes", label: "激活码", icon: "codes", render: renderCodes },
  { id: "batches", label: "批次", icon: "batches", render: renderBatches },
  { id: "devices", label: "设备", icon: "devices", render: renderDevices },
  { id: "audit", label: "操作日志", icon: "audit", render: renderAudit },
];

function currentRoute() {
  const [path, search = ""] = location.hash.replace(/^#\/?/, "").split("?");
  const route = ROUTES.find(item => item.id === path) || ROUTES[0];
  return { route, params: new URLSearchParams(search) };
}

function navigate(id, params = {}) {
  location.hash = `#/${id}${query(params)}`;
}

let shellMain = null;
let renderToken = 0;

function renderShell() {
  const nav = h("nav", { class: "nav", "aria-label": "管理导航" },
    ROUTES.map(route => h("a", { href: `#/${route.id}`, dataset: { route: route.id } }, icon(route.icon), h("span", {}, route.label))));
  const expiry = session?.expires ? `会话至 ${new Intl.DateTimeFormat("zh-CN", { timeStyle: "short" }).format(new Date(session.expires * 1000))}` : "";
  const sidebar = h("aside", { class: "sidebar" },
    h("a", { class: "brand", href: "#/overview" }, h("img", { src: "assets/icon.svg", alt: "" }),
      h("div", {}, h("strong", {}, "CCBox"), h("small", {}, "授权管理后台"))),
    nav,
    h("div", { class: "side-foot" },
      h("span", { class: "side-user" }, h("i", { "aria-hidden": "true" }, (session?.user || "?").slice(0, 1).toUpperCase()), h("span", {}, h("strong", {}, session?.user || ""), h("span", { class: "session-text" }, expiry))),
      h("button", { type: "button", onclick: logout }, "退出登录")),
  );
  shellMain = h("main", { class: "main", id: "main", tabindex: "-1" });
  app.replaceChildren(h("div", { class: "shell" }, sidebar, shellMain));
  route();
}

async function route() {
  if (!session || !shellMain) return;
  const { route: current, params } = currentRoute();
  for (const link of document.querySelectorAll(".nav a")) {
    if (link.dataset.route === current.id) link.setAttribute("aria-current", "page");
    else link.removeAttribute("aria-current");
  }
  document.title = `${current.label} · CCBox 授权管理`;
  const token = ++renderToken;
  const view = h("div", { class: "view" });
  shellMain.replaceChildren(view);
  try {
    await current.render(view, params, () => token === renderToken);
  } catch (error) {
    if (error.status === 401) return;
    if (token === renderToken) view.replaceChildren(emptyState("warn", "加载失败", error.message, h("button", { class: "btn", onclick: route }, icon("refresh"), "重试")));
  }
}

async function logout() {
  try {
    await api("logout", {});
  } catch {
    /* the session may already be gone */
  }
  session = null;
  renderLogin("已退出登录。");
}

function pageHead(title, description, ...actions) {
  return h("header", { class: "page-head" },
    h("div", {}, h("p", { class: "eyebrow" }, "CCBox 授权管理"), h("h1", {}, title), description && h("p", {}, description)),
    actions.length ? h("div", { class: "head-actions" }, actions) : null);
}

function emptyState(iconName, title, text, action) {
  return h("div", { class: "empty" }, icon(iconName), h("strong", {}, title), h("p", {}, text), action);
}

function deviceCell(device) {
  if (!device) return h("span", { class: "muted" }, "—");
  return h("span", { class: "device", title: device }, shortDevice(device),
    h("button", { class: "icon-btn", type: "button", "aria-label": "复制设备编号", onclick: () => copyText(device, "已复制设备编号") }, icon("copy")));
}

// A page beyond the end (after the last row on it was revoked, or an old
// link) moves to the last page that has rows instead of showing "no results".
function pastEnd(data, id, params) {
  const last = Math.max(1, Math.ceil(data.total / data.size));
  if (data.items.length || data.page <= last) return false;
  navigate(id, { ...params, page: last });
  return true;
}

function pager(data, onPage) {
  const pages = Math.max(1, Math.ceil(data.total / data.size));
  const from = data.total ? (data.page - 1) * data.size + 1 : 0;
  const to = Math.min(data.page * data.size, data.total);
  return h("div", { class: "pager" },
    h("span", {}, `第 ${from}–${to} 条，共 ${fmtNumber(data.total)} 条`),
    h("div", { class: "pages" },
      h("button", { class: "btn small", type: "button", disabled: data.page <= 1, onclick: () => onPage(data.page - 1) }, "上一页"),
      h("span", { style: "align-self:center" }, `${data.page} / ${pages}`),
      h("button", { class: "btn small", type: "button", disabled: data.page >= pages, onclick: () => onPage(data.page + 1) }, "下一页")));
}

function table(headers, rows) {
  return h("div", { class: "table-wrap" }, h("table", {},
    h("thead", {}, h("tr", {}, headers.map(header => h("th", { scope: "col", class: header === "" ? "actions" : undefined }, header)))),
    h("tbody", {}, rows)));
}

function chips(options, value, onChange, label) {
  const group = h("div", { class: "chips", role: "group", "aria-label": label });
  for (const [key, text] of options) {
    group.append(h("button", { type: "button", "aria-pressed": String(key === value), onclick: () => onChange(key) }, text));
  }
  return group;
}

function searchBox(value, placeholder, onSearch) {
  let timer;
  const input = h("input", { type: "search", value, placeholder, "aria-label": placeholder, oninput: () => {
    clearTimeout(timer);
    timer = setTimeout(() => onSearch(input.value.trim()), 300);
  }, onkeydown: event => {
    if (event.key === "Enter") {
      clearTimeout(timer);
      onSearch(input.value.trim());
    }
  } });
  return h("div", { class: "search" }, icon("search"), input);
}

// ------------------------------------------------------------------- Dialogs

function closeDialogs() {
  for (const dialog of document.querySelectorAll("dialog")) dialog.remove();
}

function openDialog({ title, description, body, footer, wide, onClose, canClose }) {
  const requestClose = () => {
    if (!canClose || canClose()) dialog.close();
  };
  const dialog = h("dialog", { class: wide ? "wide" : undefined, "aria-labelledby": "dialog-title" },
    h("div", { class: "dialog-head" },
      h("div", {}, h("h2", { id: "dialog-title" }, title), description && h("p", {}, description)),
      h("button", { class: "icon-btn", type: "button", "aria-label": "关闭", onclick: requestClose }, icon("close"))),
    h("div", { class: "dialog-body" }, body),
    footer && h("div", { class: "dialog-foot" }, footer));
  dialog.addEventListener("close", () => {
    dialog.remove();
    onClose?.();
  });
  dialog.addEventListener("cancel", event => {
    if (canClose && !canClose()) event.preventDefault();
  });
  dialog.addEventListener("click", event => {
    if (event.target === dialog) requestClose();
  });
  document.body.append(dialog);
  dialog.showModal();
  return dialog;
}

function confirmAction({ title, text, confirm = "确认", danger = true }) {
  return new Promise(resolve => {
    let accepted = false;
    const yes = h("button", { class: `btn ${danger ? "danger-solid" : "primary"}`, type: "button", onclick: () => {
      accepted = true;
      dialog.close();
    } }, confirm);
    const dialog = openDialog({
      title, body: h("p", { style: "margin:0" }, text),
      footer: [h("button", { class: "btn", type: "button", onclick: () => dialog.close() }, "取消"), yes],
      onClose: () => resolve(accepted),
    });
    yes.focus();
  });
}

function submitButton(label) {
  return h("button", { class: "btn primary", type: "submit" }, label);
}

async function busy(button, label, task) {
  const original = button.textContent;
  button.disabled = true;
  button.textContent = label;
  try {
    return await task();
  } finally {
    button.disabled = false;
    button.textContent = original;
  }
}

function issueDialog(onDone) {
  const count = h("input", { id: "issue-count", type: "number", min: 1, max: config.max_batch, step: 1, value: 1, required: true });
  const note = h("input", { id: "issue-note", type: "text", maxlength: config.max_note, placeholder: "例如：渠道 A · 11 月活动" });
  const error = h("p", { class: "form-error", role: "alert", hidden: true });
  const submit = submitButton("生成激活码");
  const form = h("form", { novalidate: true, onsubmit: async event => {
    event.preventDefault();
    error.hidden = true;
    const value = Number(count.value);
    if (!Number.isInteger(value) || value < 1 || value > config.max_batch) {
      error.textContent = `请输入 1 至 ${config.max_batch} 之间的整数。`;
      error.hidden = false;
      return;
    }
    try {
      const result = await busy(submit, "正在生成…", () => api("codes", { count: value, note: note.value }));
      dialog.close();
      issuedDialog(result);
      onDone?.();
    } catch (failure) {
      if (failure.status === 401) return;
      error.textContent = failure.message;
      error.hidden = false;
    }
  } },
    h("div", { class: "dialog-body" },
      h("div", { class: "row-2" },
        h("div", { class: "field" }, h("label", { for: "issue-count" }, "数量"), count),
        h("div", { class: "field" }, h("label", { for: "issue-note" }, "批次备注（可选）"), note)),
      h("p", { class: "field hint", style: "margin:0" }, `每枚激活码可为一台设备激活或续期 ${config.duration_days} 天，单批最多 ${config.max_batch} 枚。`),
      error),
    h("div", { class: "dialog-foot" }, h("button", { class: "btn", type: "button", onclick: () => dialog.close() }, "取消"), submit));
  const dialog = h("dialog", { "aria-labelledby": "issue-title" },
    h("div", { class: "dialog-head" }, h("div", {}, h("h2", { id: "issue-title" }, "生成激活码"), h("p", {}, "生成后仅显示一次，服务端只保存哈希。")),
      h("button", { class: "icon-btn", type: "button", "aria-label": "关闭", onclick: () => dialog.close() }, icon("close"))),
    form);
  dialog.addEventListener("close", () => dialog.remove());
  document.body.append(dialog);
  dialog.showModal();
  count.select();
}

function issuedDialog(result) {
  const text = result.codes.join("\n");
  const area = h("textarea", { class: "code-out", readonly: true, wrap: "off", rows: Math.min(10, result.codes.length + 1), "aria-label": "已生成的激活码" }, text);
  const filename = `ccbox-codes-${result.batch}`;
  let saved = false;
  const mark = () => { saved = true; };
  openDialog({
    title: `已生成 ${result.codes.length} 枚激活码`,
    description: `批次 ${result.batch}${result.note ? ` · ${result.note}` : ""}`,
    wide: true,
    canClose: () => saved || window.confirm("尚未复制或下载激活码，关闭后将无法再次查看。确定关闭？"),
    body: [
      h("div", { class: "notice" }, icon("warn"), h("span", {}, "激活码只在此显示一次。关闭前请复制或下载，之后后台只能查询状态，无法再次查看原码。")),
      area,
    ],
    footer: [
      h("button", { class: "btn left", type: "button", onclick: () => { mark(); download(`${filename}.csv`, toCSV([["激活码", "批次", "备注", "生成时间"], ...result.codes.map(code => [code, result.batch, result.note, fmtTime(result.created)])]), "text/csv"); } }, icon("download"), "下载 CSV"),
      h("button", { class: "btn", type: "button", onclick: () => { mark(); download(`${filename}.txt`, text); } }, icon("download"), "下载 TXT"),
      h("button", { class: "btn primary", type: "button", onclick: () => { mark(); copyText(text, `已复制 ${result.codes.length} 枚激活码`); } }, icon("copy"), "复制全部"),
    ],
  });
  area.addEventListener("copy", mark);
  area.focus();
  area.select();
}

function singleCodeDialog({ title, description, code, details }) {
  openDialog({
    title, description,
    body: [
      h("div", { class: "code-single" }, h("span", {}, code),
        h("button", { class: "btn small", type: "button", onclick: () => copyText(code) }, icon("copy"), "复制")),
      details,
      h("div", { class: "notice info" }, icon("info"), h("span", {}, "此码只能由指定设备在「设置 → 授权」中兑换，仅显示一次。")),
    ],
    footer: [h("button", { class: "btn primary", type: "button", onclick: event => event.target.closest("dialog").close() }, "完成")],
  });
}

function deviceInput(id, label, value = "") {
  return h("div", { class: "field" }, h("label", { for: id }, label),
    h("input", { id, type: "text", value, required: true, minlength: 64, maxlength: 64, pattern: "[0-9a-fA-F]{64}", spellcheck: "false", autocomplete: "off", class: "mono", placeholder: "64 位十六进制设备编号" }));
}

function transferDialog(from = "", onDone) {
  const error = h("p", { class: "form-error", role: "alert", hidden: true });
  const submit = submitButton("生成换机码");
  const fromField = deviceInput("transfer-from", "原设备编号", from);
  const toField = deviceInput("transfer-to", "新设备编号");
  const dialog = openDialog({
    title: "换机",
    description: "保留剩余有效期，把授权迁移到新设备。",
    body: h("form", { id: "transfer-form", novalidate: true, style: "display:grid;gap:14px", onsubmit: async event => {
      event.preventDefault();
      error.hidden = true;
      try {
        const result = await busy(submit, "正在生成…", () => api("devices/transfer", {
          from: fromField.querySelector("input").value, to: toField.querySelector("input").value,
        }));
        dialog.close();
        singleCodeDialog({ title: "换机码已生成", description: "发给授权持有人，在新设备上输入。", code: result.code,
          details: h("dl", { class: "kv" }, h("dt", {}, "新设备"), h("dd", { class: "mono" }, result.device)) });
        onDone?.();
      } catch (failure) {
        if (failure.status === 401) return;
        error.textContent = failure.message;
        error.hidden = false;
      }
    } }, fromField, toField,
      h("div", { class: "notice" }, icon("warn"), h("span", {}, "要求原设备仍在有效期内、新设备当前无有效授权。原设备已保存的离线凭证会在原到期时间失效。")),
      error),
    footer: [h("button", { class: "btn", type: "button", onclick: () => dialog.close() }, "取消"), submit],
  });
  submit.setAttribute("form", "transfer-form");
  (from ? toField : fromField).querySelector("input").focus();
}

function renewDialog(device = "", onDone) {
  const error = h("p", { class: "form-error", role: "alert", hidden: true });
  const submit = submitButton(`续期 ${config.duration_days} 天`);
  const field = deviceInput("renew-device", "设备编号", device);
  const dialog = openDialog({
    title: "设备续期",
    description: `为指定设备增加 ${config.duration_days} 天，生成仅该设备可用的续期码。适用于补偿或人工续费。`,
    body: h("form", { id: "renew-form", novalidate: true, style: "display:grid;gap:14px", onsubmit: async event => {
      event.preventDefault();
      error.hidden = true;
      try {
        const result = await busy(submit, "正在生成…", () => api("devices/renew", { device: field.querySelector("input").value }));
        dialog.close();
        singleCodeDialog({ title: "续期码已生成", description: "账本中的到期时间已更新，设备兑换后即可离线使用新期限。", code: result.code,
          details: h("dl", { class: "kv" }, h("dt", {}, "设备"), h("dd", { class: "mono" }, result.device), h("dt", {}, "新到期时间"), h("dd", {}, fmtTime(result.expires))) });
        onDone?.();
      } catch (failure) {
        if (failure.status === 401) return;
        error.textContent = failure.message;
        error.hidden = false;
      }
    } }, field, error),
    footer: [h("button", { class: "btn", type: "button", onclick: () => dialog.close() }, "取消"), submit],
  });
  submit.setAttribute("form", "renew-form");
  field.querySelector("input").focus();
}

function codeDetails(code) {
  const status = CODE_STATUS[code.status] || [code.status, ""];
  return h("dl", { class: "kv" },
    h("dt", {}, "状态"), h("dd", {}, badge(status)),
    h("dt", {}, "类型"), h("dd", {}, CODE_KIND[code.kind] || code.kind),
    h("dt", {}, "尾号"), h("dd", { class: "mono" }, code.hint ? `…${code.hint}` : "—"),
    h("dt", {}, "批次"), h("dd", {}, code.batch || "—"),
    h("dt", {}, "生成时间"), h("dd", {}, fmtTime(code.issued)),
    h("dt", {}, "兑换时间"), h("dd", {}, fmtTime(code.redeemed)),
    code.revoked ? [h("dt", {}, "停用时间"), h("dd", {}, fmtTime(code.revoked))] : null,
    h("dt", {}, "设备"), h("dd", { class: "mono" }, code.device || "—"),
    code.from ? [h("dt", {}, "原设备"), h("dd", { class: "mono" }, code.from)] : null,
    h("dt", {}, "授权到期"), h("dd", {}, code.expires ? `${fmtTime(code.expires)}（${remaining(code.expires)}）` : "—"));
}

function lookupDialog(onChange) {
  const input = h("input", { id: "lookup-code", type: "text", required: true, maxlength: 64, autocomplete: "off", spellcheck: "false", class: "mono", placeholder: "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX" });
  const result = h("div");
  const submit = submitButton("查询");
  const dialog = openDialog({
    title: "查询激活码",
    description: "输入用户提供的完整激活码，查看兑换状态和绑定设备。",
    wide: true,
    body: [h("form", { id: "lookup-form", novalidate: true, style: "display:grid;gap:12px", onsubmit: async event => {
      event.preventDefault();
      result.replaceChildren();
      try {
        const code = await busy(submit, "正在查询…", () => api("codes/lookup", { code: input.value }));
        const actions = code.status === "unused" ? h("div", { style: "margin-top:14px" }, h("button", { class: "btn danger small", type: "button", onclick: async () => {
          if (await revoke(code)) {
            dialog.close();
            onChange?.();
          }
        } }, icon("ban"), "停用此码")) : null;
        result.replaceChildren(h("div", { class: "card", style: "padding:16px;box-shadow:none" }, codeDetails(code), actions));
      } catch (failure) {
        if (failure.status === 401) return;
        result.replaceChildren(h("p", { class: "form-error", role: "alert" }, failure.message));
      }
    } }, h("div", { class: "field" }, h("label", { for: "lookup-code" }, "激活码"), input)), result],
    footer: [h("button", { class: "btn", type: "button", onclick: () => dialog.close() }, "关闭"), submit],
  });
  submit.setAttribute("form", "lookup-form");
  input.focus();
}

async function revoke(code) {
  const ok = await confirmAction({ title: "停用激活码？", text: `停用后尾号 …${code.hint || "—"} 的激活码将无法兑换，此操作不可撤销。`, confirm: "停用" });
  if (!ok) return false;
  try {
    await api("codes/revoke", { id: code.id });
    toast("激活码已停用");
    return true;
  } catch (failure) {
    if (failure.status !== 401) toast(failure.message, "error");
    return false;
  }
}

// ------------------------------------------------------------------ Overview

function stat(label, value, sub, color) {
  return h("div", { class: "card stat", style: color ? `--dot:${color}` : undefined },
    h("div", { class: "label" }, h("i"), label),
    h("div", { class: "value" }, fmtNumber(value)),
    sub && h("div", { class: "sub" }, sub));
}

function barChart(series) {
  const narrow = window.innerWidth < 760;
  const width = narrow ? 340 : 640;
  const height = narrow ? 200 : 220;
  const pad = { top: 12, right: 8, bottom: 24, left: 30 };
  const top = Math.max(4, ...series.map(day => Math.max(day.issued, day.redeemed)));
  const nice = Math.ceil(top / 4) * 4;
  const inner = { w: width - pad.left - pad.right, h: height - pad.top - pad.bottom };
  const slot = inner.w / series.length;
  const barW = Math.max(2, slot / 2 - 1.5);
  const svg = document.createElementNS(SVG, "svg");
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);
  svg.setAttribute("class", "chart");
  svg.setAttribute("role", "img");
  const total = series.reduce((sum, day) => sum + day.redeemed, 0);
  svg.setAttribute("aria-label", `近 30 天共兑换 ${total} 枚激活码`);
  const add = (tag, attrs, text) => {
    const element = document.createElementNS(SVG, tag);
    for (const [key, value] of Object.entries(attrs)) element.setAttribute(key, value);
    if (text !== undefined) element.textContent = text;
    svg.append(element);
    return element;
  };
  for (let i = 0; i <= 4; i++) {
    const value = (nice / 4) * i;
    const y = pad.top + inner.h - (value / nice) * inner.h;
    add("line", { x1: pad.left, x2: width - pad.right, y1: y, y2: y, class: "gridline" });
    add("text", { x: pad.left - 6, y: y + 3, "text-anchor": "end", class: "axis" }, String(value));
  }
  series.forEach((day, index) => {
    const x = pad.left + index * slot + (slot - barW * 2 - 1) / 2;
    for (const [key, offset, cls] of [["issued", 0, "bar-issued"], ["redeemed", barW + 1, "bar-redeemed"]]) {
      const value = day[key];
      const barH = (value / nice) * inner.h;
      const rect = add("rect", { x: x + offset, y: pad.top + inner.h - barH, width: barW, height: Math.max(barH, value ? 1.5 : 0), rx: 1.5, class: cls });
      const title = document.createElementNS(SVG, "title");
      title.textContent = `${day.day} ${key === "issued" ? "生成" : "兑换"} ${value}`;
      rect.append(title);
    }
    if (index % (narrow ? 10 : 5) === 0 || index === series.length - 1) {
      add("text", { x: pad.left + index * slot + slot / 2, y: height - 6, "text-anchor": "middle", class: "axis" }, day.day.slice(5));
    }
  });
  return svg;
}

async function renderOverview(view, _params, alive) {
  const data = await api("overview");
  if (!alive()) return;
  const codes = data.codes || {};
  const devices = data.devices || {};
  const series = data.series || [];
  const issued30 = series.reduce((sum, day) => sum + day.issued, 0);
  const redeemed30 = series.reduce((sum, day) => sum + day.redeemed, 0);
  const rate = codes.issued ? Math.round(((codes.redeemed || 0) / codes.issued) * 100) : 0;
  const reload = () => route();
  view.replaceChildren(
    pageHead("概览", `数据更新于 ${fmtTime(data.generated)}`,
      h("button", { class: "btn", type: "button", onclick: reload }, icon("refresh"), "刷新"),
      h("button", { class: "btn", type: "button", onclick: () => transferDialog("", reload) }, icon("transfer"), "换机"),
      h("button", { class: "btn primary", type: "button", onclick: () => issueDialog(reload) }, icon("plus"), "生成激活码")),
    h("section", { class: "stats", "aria-label": "关键指标" },
      stat("注册用户", (data.users || {}).total, `近 30 天新增 ${fmtNumber((data.users || {}).new30)} · 有效 ${fmtNumber((data.users || {}).active)}`, "var(--brand)"),
      stat("累计发放", codes.issued, `近 30 天 ${fmtNumber(issued30)}`, "var(--muted)"),
      stat("未使用", codes.unused, "可兑换库存", "var(--info)"),
      stat("已兑换", codes.redeemed, `兑换率 ${rate}%`, "var(--ok)"),
      stat("已停用", codes.revoked, `换机 ${fmtNumber(codes.transfer)} · 续期 ${fmtNumber(codes.renewal)}`, "var(--bad)"),
      stat("有效设备", devices.active, `到期 ${fmtNumber(devices.expired)} · 转出 ${fmtNumber(devices.transferred)}`, "var(--brand)"),
      stat("7 天内到期", data.expiring_total ?? (data.expiring || []).length, "可提醒续期", "var(--warn)")),
    h("div", { class: "grid-2" },
      h("section", { class: "card" },
        h("div", { class: "card-head" }, h("h2", {}, "近 30 天"),
          h("div", { class: "legend" }, h("span", { style: "--c:var(--line-2)" }, h("i"), `生成 ${fmtNumber(issued30)}`), h("span", { style: "--c:var(--brand)" }, h("i"), `兑换 ${fmtNumber(redeemed30)}`))),
        h("div", { class: "card-body" }, barChart(series))),
      h("section", { class: "card" },
        h("div", { class: "card-head" }, h("h2", {}, "即将到期"), h("a", { href: "#/devices?status=active" }, "全部设备")),
        h("div", { class: "card-body" }, (data.expiring || []).length ? h("ul", { class: "list" }, data.expiring.map(device =>
          h("li", {}, h("div", {}, h("div", { class: "primary-text" }, deviceCell(device.device)), h("div", { class: "secondary-text" }, `${fmtTime(device.expires)} · ${remaining(device.expires)}`)),
            h("button", { class: "btn small", type: "button", onclick: () => renewDialog(device.device, reload) }, "续期"))))
          : h("p", { class: "empty-mini" }, "7 天内没有到期的设备")))),
    h("div", { class: "grid-2" },
      h("section", { class: "card" },
        h("div", { class: "card-head" }, h("h2", {}, "最近批次"), h("a", { href: "#/batches" }, "全部批次")),
        (data.batches || []).length ? batchTable(data.batches, reload, true) : h("div", { class: "card-body" }, h("p", { class: "empty-mini" }, "还没有通过后台生成的批次"))),
      h("section", { class: "card" },
        h("div", { class: "card-head" }, h("h2", {}, "快捷操作")),
        h("div", { class: "card-body" }, h("ul", { class: "list" },
          h("li", {}, h("div", {}, h("div", { class: "primary-text" }, "查询激活码"), h("div", { class: "secondary-text" }, "核对用户反馈的码是否已被兑换")), h("button", { class: "btn small", type: "button", onclick: () => lookupDialog(reload) }, "查询")),
          h("li", {}, h("div", {}, h("div", { class: "primary-text" }, "设备续期"), h("div", { class: "secondary-text" }, `补偿或人工续费 ${config.duration_days} 天`)), h("button", { class: "btn small", type: "button", onclick: () => renewDialog("", reload) }, "续期")),
          h("li", {}, h("div", {}, h("div", { class: "primary-text" }, "换机"), h("div", { class: "secondary-text" }, "保留剩余期限迁移到新设备")), h("button", { class: "btn small", type: "button", onclick: () => transferDialog("", reload) }, "换机")),
          h("li", {}, h("div", {}, h("div", { class: "primary-text" }, "操作日志"), h("div", { class: "secondary-text" }, "登录、发码、停用与换机记录")), h("a", { class: "btn small", href: "#/audit" }, "查看")))))),
  );
}

// --------------------------------------------------------------------- Codes

async function fetchAll(path, params) {
  const items = [];
  for (let page = 1; ; page++) {
    const data = await api(path + query({ ...params, page, size: 200 }));
    items.push(...data.items);
    if (items.length >= data.total || !data.items.length) return items;
  }
}

async function renderCodes(view, params, alive) {
  const filters = { status: params.get("status") || "", kind: params.get("kind") || "", batch: params.get("batch") || "", q: params.get("q") || "" };
  const page = Number(params.get("page")) || 1;
  const update = changes => navigate("codes", { ...filters, page: 1, ...changes });
  const data = await api("codes" + query({ ...filters, page }));
  if (!alive() || pastEnd(data, "codes", filters)) return;
  const reload = () => route();
  const exportCSV = async event => {
    const items = await busy(event.currentTarget, "正在导出…", () => fetchAll("codes", filters));
    download(`ccbox-code-records-${stamp()}.csv`, toCSV([["记录编号", "尾号", "类型", "状态", "批次", "生成时间", "兑换时间", "停用时间", "设备", "原设备", "授权到期"],
      ...items.map(code => [code.id, code.hint, CODE_KIND[code.kind] || code.kind, CODE_STATUS[code.status]?.[0] || code.status, code.batch, fmtTime(code.issued), fmtTime(code.redeemed), fmtTime(code.revoked), code.device, code.from, fmtTime(code.expires)])]), "text/csv");
  };
  const rows = data.items.map(code => h("tr", {},
    h("td", { class: "mono" }, code.hint ? `…${code.hint}` : h("span", { class: "muted" }, "旧记录")),
    h("td", {}, h("span", { class: "tag" }, CODE_KIND[code.kind] || code.kind)),
    h("td", {}, badge(CODE_STATUS[code.status] || [code.status, ""])),
    h("td", {}, code.batch ? h("a", { href: `#/codes${query({ batch: code.batch })}` }, code.batch) : h("span", { class: "muted" }, "—")),
    h("td", { title: fmtTime(code.issued) }, fmtShort(code.issued)),
    h("td", { title: fmtTime(code.redeemed) }, fmtShort(code.redeemed)),
    h("td", {}, deviceCell(code.device)),
    h("td", {}, code.expires ? h("span", { title: remaining(code.expires) }, fmtDate(code.expires)) : h("span", { class: "muted" }, "—")),
    h("td", { class: "actions" },
      h("button", { class: "btn ghost small", type: "button", onclick: () => openDialog({ title: "激活码详情", body: codeDetails(code), wide: true }) }, "详情"),
      code.status === "unused" && h("button", { class: "btn danger small", type: "button", onclick: async () => { if (await revoke(code)) reload(); } }, "停用"))));
  const filtered = Object.values(filters).some(Boolean);
  view.replaceChildren(
    pageHead("激活码", "每个码记录类型、批次、兑换设备与到期时间；原码不在服务器保存。",
      h("button", { class: "btn", type: "button", onclick: () => lookupDialog(reload) }, icon("search"), "查询完整激活码"),
      h("button", { class: "btn primary", type: "button", onclick: () => issueDialog(reload) }, icon("plus"), "生成激活码")),
    h("div", { class: "toolbar" },
      searchBox(filters.q, "尾号、设备编号或批次", q => update({ q })),
      chips([["", "全部"], ["unused", "未使用"], ["redeemed", "已兑换"], ["pending", "待兑换"], ["revoked", "已停用"]], filters.status, status => update({ status }), "状态"),
      h("select", { "aria-label": "类型", onchange: event => update({ kind: event.target.value }) },
        [["", "全部类型"], ["code", "激活码"], ["transfer", "换机码"], ["renewal", "续期码"]].map(([value, label]) => h("option", { value, selected: value === filters.kind }, label))),
      filters.batch && h("span", { class: "tag" }, `批次 ${filters.batch} `, h("button", { class: "icon-btn", type: "button", "aria-label": "清除批次筛选", onclick: () => update({ batch: "" }) }, icon("close"))),
      h("span", { class: "spacer" }),
      h("button", { class: "btn", type: "button", onclick: exportCSV, disabled: !data.total }, icon("download"), "导出 CSV")),
    h("section", { class: "card" },
      data.items.length ? [table(["尾号", "类型", "状态", "批次", "生成时间", "兑换时间", "设备", "到期", ""], rows), pager(data, next => navigate("codes", { ...filters, page: next }))]
        : filtered ? emptyState("search", "没有匹配的记录", "调整筛选条件后重试。", h("button", { class: "btn", type: "button", onclick: () => navigate("codes") }, "清除筛选"))
          : emptyState("inbox", "还没有激活码", "生成第一批激活码后，会在这里看到兑换进度。", h("button", { class: "btn primary", type: "button", onclick: () => issueDialog(reload) }, icon("plus"), "生成激活码"))),
  );
}

// ------------------------------------------------------------------- Batches

function batchTable(batches, reload, compact = false) {
  const rows = batches.map(batch => {
    const total = batch.count || batch.unused + batch.redeemed + batch.revoked || 1;
    return h("tr", {},
      h("td", {}, h("a", { class: "mono", href: `#/codes${query({ batch: batch.id })}` }, batch.id)),
      h("td", { class: "wrap" }, batch.note || h("span", { class: "muted" }, "—")),
      !compact && h("td", { title: fmtTime(batch.created) }, fmtShort(batch.created)),
      h("td", {}, fmtNumber(batch.count)),
      h("td", {}, h("div", { style: "display:flex;align-items:center;gap:10px" },
        h("div", { class: "progress", title: `已兑换 ${batch.redeemed} · 未使用 ${batch.unused} · 已停用 ${batch.revoked}` },
          h("span", { class: "p-ok", style: `width:${(batch.redeemed / total) * 100}%` }),
          h("span", { class: "p-off", style: `width:${(batch.revoked / total) * 100}%` })),
        h("span", { class: "muted" }, `${batch.redeemed}/${batch.count}`))),
      !compact && h("td", {}, fmtNumber(batch.unused)),
      !compact && h("td", { class: "actions" },
        h("a", { class: "btn ghost small", href: `#/codes${query({ batch: batch.id })}` }, "查看"),
        batch.unused > 0 && h("button", { class: "btn danger small", type: "button", onclick: async () => {
          const ok = await confirmAction({ title: "停用整批未使用的激活码？", text: `批次 ${batch.id} 中 ${batch.unused} 枚未使用的激活码将无法兑换，已兑换的不受影响。`, confirm: "停用未使用的码" });
          if (!ok) return;
          try {
            const result = await api("batches/revoke", { id: batch.id });
            toast(`已停用 ${result.revoked} 枚激活码`);
            reload();
          } catch (failure) {
            if (failure.status !== 401) toast(failure.message, "error");
          }
        } }, "停用未使用")));
  });
  const headers = compact ? ["批次", "备注", "数量", "兑换进度"] : ["批次", "备注", "创建时间", "数量", "兑换进度", "未使用", ""];
  return table(headers, rows);
}

async function renderBatches(view, params, alive) {
  const q = params.get("q") || "";
  const page = Number(params.get("page")) || 1;
  const data = await api("batches" + query({ q, page }));
  if (!alive() || pastEnd(data, "batches", { q })) return;
  const reload = () => route();
  view.replaceChildren(
    pageHead("批次", "按发放渠道或活动管理激活码，可一键停用整批未使用的码。",
      h("button", { class: "btn primary", type: "button", onclick: () => issueDialog(reload) }, icon("plus"), "新建批次")),
    h("div", { class: "toolbar" }, searchBox(q, "批次编号或备注", value => navigate("batches", { q: value }))),
    h("section", { class: "card" }, data.items.length
      ? [batchTable(data.items, reload), pager(data, next => navigate("batches", { q, page: next }))]
      : q ? emptyState("search", "没有匹配的批次", "换个关键词试试。")
        : emptyState("batches", "还没有批次", "在后台生成的每一批激活码都会记录为一个批次；命令行生成的码不归入批次。", h("button", { class: "btn primary", type: "button", onclick: () => issueDialog(reload) }, icon("plus"), "新建批次"))),
  );
}

// ------------------------------------------------------------------- Devices

async function renderDevices(view, params, alive) {
  const filters = { status: params.get("status") || "", q: params.get("q") || "" };
  const page = Number(params.get("page")) || 1;
  const data = await api("devices" + query({ ...filters, page }));
  if (!alive() || pastEnd(data, "devices", filters)) return;
  const reload = () => route();
  const update = changes => navigate("devices", { ...filters, ...changes });
  const exportCSV = async event => {
    const items = await busy(event.currentTarget, "正在导出…", () => fetchAll("devices", filters));
    download(`ccbox-devices-${stamp()}.csv`, toCSV([["设备编号", "状态", "到期时间", "兑换次数", "首次激活", "最近兑换"],
      ...items.map(device => [device.device, DEVICE_STATUS[device.status]?.[0] || device.status, fmtTime(device.expires), device.codes, fmtTime(device.first_seen), fmtTime(device.last_seen)])]), "text/csv");
  };
  const rows = data.items.map(device => {
    const soon = device.status === "active" && device.expires - nowSeconds() < 7 * 86400;
    return h("tr", {},
      h("td", {}, deviceCell(device.device)),
      h("td", {}, badge(soon ? ["即将到期", "warn"] : DEVICE_STATUS[device.status] || [device.status, ""])),
      h("td", { title: fmtTime(device.expires) }, fmtShort(device.expires)),
      h("td", { class: "muted" }, device.expires ? remaining(device.expires) : "—"),
      h("td", {}, fmtNumber(device.codes)),
      h("td", {}, fmtDate(device.first_seen)),
      h("td", {}, fmtDate(device.last_seen)),
      h("td", { class: "actions" },
        h("a", { class: "btn ghost small", href: `#/codes${query({ q: device.device })}` }, "记录"),
        device.status !== "transferred" && h("button", { class: "btn small", type: "button", onclick: () => renewDialog(device.device, reload) }, "续期"),
        device.status === "active" && h("button", { class: "btn small", type: "button", onclick: () => transferDialog(device.device, reload) }, "换机")));
  });
  const filtered = Object.values(filters).some(Boolean);
  view.replaceChildren(
    pageHead("设备", "设备编号来自客户端「设置 → 授权」，是产品范围内的哈希，不含原始系统标识。",
      h("button", { class: "btn", type: "button", onclick: () => renewDialog("", reload) }, icon("renew"), "续期"),
      h("button", { class: "btn primary", type: "button", onclick: () => transferDialog("", reload) }, icon("transfer"), "换机")),
    h("div", { class: "toolbar" },
      searchBox(filters.q, "设备编号前缀", q => update({ q })),
      chips([["", "全部"], ["active", "有效"], ["expired", "已到期"], ["transferred", "已转出"]], filters.status, status => update({ status }), "状态"),
      h("span", { class: "spacer" }),
      h("button", { class: "btn", type: "button", onclick: exportCSV, disabled: !data.total }, icon("download"), "导出 CSV")),
    h("section", { class: "card" }, data.items.length
      ? [table(["设备", "状态", "到期时间", "剩余", "兑换次数", "首次激活", "最近兑换", ""], rows), pager(data, next => update({ page: next }))]
      : filtered ? emptyState("search", "没有匹配的设备", "调整筛选条件后重试。", h("button", { class: "btn", type: "button", onclick: () => navigate("devices") }, "清除筛选"))
        : emptyState("devices", "还没有激活的设备", "用户兑换激活码后，设备会出现在这里。")),
  );
}

// --------------------------------------------------------------------- Users

function userExpiry(user) {
  if (!user.expires) return h("span", { class: "muted" }, "—");
  return h("span", { title: fmtTime(user.expires) }, fmtDate(user.expires), h("span", { class: "muted" }, ` · ${remaining(user.expires)}`));
}

async function renderUsers(view, params, alive) {
  const filters = { status: params.get("status") || "", q: params.get("q") || "" };
  const page = Number(params.get("page")) || 1;
  const data = await api("users" + query({ ...filters, page }));
  if (!alive() || pastEnd(data, "users", filters)) return;
  const reload = () => route();
  const update = changes => navigate("users", { ...filters, page: 1, ...changes });
  const totals = data.totals || {};
  const exportCSV = async event => {
    const items = await busy(event.currentTarget, "正在导出…", () => fetchAll("users", filters));
    download(`ccbox-users-${stamp()}.csv`, toCSV([["用户编号", "用户名", "状态", "授权到期", "设备数", "注册时间", "最近登录", "停用原因"],
      ...items.map(user => [user.id, user.username, USER_STATUS[user.status]?.[0] || user.status, fmtTime(user.expires), `${user.device_count}/${user.max_devices}`, fmtTime(user.created), fmtTime(user.last_login), user.disabled_reason])]), "text/csv");
  };
  const rows = data.items.map(user => h("tr", {},
    h("td", {}, h("div", { class: "primary-text" }, user.username), h("div", { class: "secondary-text mono" }, user.id.slice(0, 12))),
    h("td", {}, badge(USER_STATUS[user.status] || [user.status, ""])),
    h("td", {}, userExpiry(user)),
    h("td", {}, `${user.device_count} / ${user.max_devices}`),
    h("td", { title: fmtTime(user.created) }, fmtShort(user.created)),
    h("td", { title: fmtTime(user.last_login) }, fmtShort(user.last_login)),
    h("td", { class: "actions" }, h("button", { class: "btn ghost small", type: "button", onclick: () => userDialog(user.id, reload) }, "详情"))));
  const filtered = Object.values(filters).some(Boolean);
  view.replaceChildren(
    pageHead("用户", "线上账户与授权期限。密码只以哈希保存，后台无法查看或直接设置用户密码。"),
    h("section", { class: "stats", "aria-label": "用户统计" },
      stat("注册用户", totals.total, `近 30 天新增 ${fmtNumber(totals.new30)}`, "var(--brand)"),
      stat("授权有效", totals.active, `7 天内到期 ${fmtNumber(totals.expiring)}`, "var(--ok)"),
      stat("已到期", totals.expired, "可提醒续期", "var(--warn)"),
      stat("未授权", totals.none, "注册后尚未兑换", "var(--info)"),
      stat("已停用", totals.disabled, "停用后无法登录", "var(--bad)")),
    h("div", { class: "toolbar" },
      searchBox(filters.q, "用户名或用户编号", q => update({ q })),
      chips([["", "全部"], ["active", "有效"], ["expired", "已到期"], ["none", "未授权"], ["disabled", "已停用"]], filters.status, status => update({ status }), "状态"),
      h("span", { class: "spacer" }),
      h("button", { class: "btn", type: "button", onclick: exportCSV, disabled: !data.total }, icon("download"), "导出 CSV")),
    h("section", { class: "card" }, data.items.length
      ? [table(["用户", "状态", "授权到期", "设备", "注册时间", "最近登录", ""], rows), pager(data, next => navigate("users", { ...filters, page: next }))]
      : filtered ? emptyState("search", "没有匹配的用户", "调整筛选条件后重试。", h("button", { class: "btn", type: "button", onclick: () => navigate("users") }, "清除筛选"))
        : emptyState("users", "还没有注册用户", "用户在桌面应用中注册后会出现在这里。")),
  );
}

// A small form dialog: fields plus a required reason, posting to path.
function userActionDialog({ title, description, fields = [], reasonLabel = "原因", reasonRequired = true, confirm, danger, onSubmit }) {
  const reason = h("input", { id: "user-reason", type: "text", maxlength: 80, placeholder: "写入操作日志，例如：工单 1234" });
  const error = h("p", { class: "form-error", role: "alert", hidden: true });
  const submit = h("button", { class: `btn ${danger ? "danger-solid" : "primary"}`, type: "submit", form: "user-action-form" }, confirm);
  const dialog = openDialog({
    title, description,
    body: h("form", { id: "user-action-form", novalidate: true, style: "display:grid;gap:14px", onsubmit: async event => {
      event.preventDefault();
      error.hidden = true;
      if (reasonRequired && !reason.value.trim()) {
        error.textContent = `请填写${reasonLabel}`;
        error.hidden = false;
        return;
      }
      try {
        await busy(submit, "正在处理…", () => onSubmit(reason.value.trim()));
        dialog.close();
      } catch (failure) {
        if (failure.status === 401) return;
        error.textContent = failure.message;
        error.hidden = false;
      }
    } }, fields, h("div", { class: "field" }, h("label", { for: "user-reason" }, reasonRequired ? reasonLabel : `${reasonLabel}（可选）`), reason), error),
    footer: [h("button", { class: "btn", type: "button", onclick: () => dialog.close() }, "取消"), submit],
  });
  (fields[0]?.querySelector?.("input") || reason).focus();
}

function resetCodeDialog(result) {
  openDialog({
    title: "重置码已生成",
    description: `用户 ${result.user.username} · 有效至 ${fmtTime(result.expires)}`,
    body: [
      h("div", { class: "code-single" }, h("span", {}, result.code),
        h("button", { class: "btn small", type: "button", onclick: () => copyText(result.code) }, icon("copy"), "复制")),
      h("div", { class: "notice info" }, icon("info"), h("span", {}, "请通过可信渠道交给用户本人。用户在桌面应用登录页点击「忘记密码」，输入用户名、重置码和新密码。重置码只能使用一次，仅显示这一次。")),
    ],
    footer: [h("button", { class: "btn primary", type: "button", onclick: event => event.target.closest("dialog").close() }, "完成")],
  });
}

async function userDialog(id, onChange) {
  let data;
  try {
    data = await api("users/detail" + query({ id }));
  } catch (failure) {
    if (failure.status !== 401) toast(failure.message, "error");
    return;
  }
  const user = data.user;
  const refresh = () => {
    dialog.close();
    onChange?.();
    userDialog(id, onChange);
  };
  const devices = (user.devices || []).length
    ? table(["设备", "名称", "系统", "首次登录", "最近使用", ""], user.devices.map(device => h("tr", {},
      h("td", {}, deviceCell(device.id)),
      h("td", {}, device.name || h("span", { class: "muted" }, "—")),
      h("td", {}, device.platform || h("span", { class: "muted" }, "—")),
      h("td", { title: fmtTime(device.added) }, fmtShort(device.added)),
      h("td", { title: fmtTime(device.seen) }, fmtShort(device.seen)),
      h("td", { class: "actions" }, h("button", { class: "btn danger small", type: "button", onclick: () => userActionDialog({
        title: "解绑设备？", description: `设备 ${shortDevice(device.id)} 将退出登录并释放一个设备名额；已签发的离线授权最长 7 天后失效。`,
        reasonRequired: false, confirm: "解绑", danger: true,
        onSubmit: async reason => { await api("users/devices/remove", { id, device: device.id, reason }); toast("设备已解绑"); refresh(); },
      }) }, "解绑")))))
    : h("p", { class: "empty-mini" }, "当前没有登录的设备");
  const events = (user.events || []).length
    ? h("ul", { class: "list" }, user.events.map(event => h("li", {},
      h("div", {}, h("div", { class: "primary-text" }, EVENT_SOURCE[event.source] || event.source,
        event.seconds ? h("span", { class: "muted" }, ` ${event.seconds > 0 ? "+" : ""}${Math.round(event.seconds / 86400)} 天`) : null),
        h("div", { class: "secondary-text" }, `${fmtTime(event.time)} · 到期 ${fmtDate(event.expires)}${event.note ? ` · ${event.note}` : ""}${event.ref ? ` · ${event.ref}` : ""}`)))))
    : h("p", { class: "empty-mini" }, "还没有授权记录");
  const audit = (data.audit || []).length
    ? h("ul", { class: "list" }, data.audit.map(entry => h("li", {}, h("div", {},
      h("div", { class: "primary-text" }, AUDIT_ACTION[entry.action] || entry.action),
      h("div", { class: "secondary-text" }, `${fmtTime(entry.time)} · ${entry.detail || ""}`)))))
    : h("p", { class: "empty-mini" }, "没有相关的后台操作");
  const disabled = user.status === "disabled";
  const dialog = openDialog({
    title: user.username,
    description: `用户编号 ${user.id}`,
    wide: true,
    body: [
      h("dl", { class: "kv" },
        h("dt", {}, "状态"), h("dd", {}, badge(USER_STATUS[user.status] || [user.status, ""]), disabled && user.disabled_reason ? h("span", { class: "muted" }, ` ${user.disabled_reason}`) : null),
        h("dt", {}, "授权到期"), h("dd", {}, user.expires ? `${fmtTime(user.expires)}（${remaining(user.expires)}）` : "—"),
        h("dt", {}, "设备"), h("dd", {}, `${user.device_count} / ${user.max_devices} · 有效会话 ${user.sessions}`),
        h("dt", {}, "注册时间"), h("dd", {}, fmtTime(user.created)),
        h("dt", {}, "最近登录"), h("dd", {}, fmtTime(user.last_login)),
        user.reset_pending ? [h("dt", {}, "重置码"), h("dd", {}, "已发放，尚未使用")] : null),
      h("h3", { class: "section-title" }, "登录的设备"), devices,
      h("h3", { class: "section-title" }, "授权记录"), events,
      h("h3", { class: "section-title" }, "后台操作"), audit,
    ],
    footer: [
      h("button", { class: `btn left ${disabled ? "" : "danger"}`, type: "button", onclick: () => disabled
        ? userActionDialog({ title: "恢复此账户？", description: "恢复后用户可以重新登录。", reasonRequired: false, confirm: "恢复账户",
          onSubmit: async reason => { await api("users/enable", { id, reason }); toast("账户已恢复"); refresh(); } })
        : userActionDialog({ title: "停用此账户？", description: "所有设备立即退出登录，无法再登录或续签授权；已签发的离线授权最长 7 天后失效。", confirm: "停用账户", danger: true,
          onSubmit: async reason => { await api("users/disable", { id, reason }); toast("账户已停用"); refresh(); } }) },
        disabled ? "恢复账户" : "停用账户"),
      h("button", { class: "btn", type: "button", onclick: () => userActionDialog({
        title: "发放密码重置码", description: "生成一次性重置码，24 小时内有效。原密码在用户完成重置前仍然可用。", reasonRequired: false, confirm: "生成重置码",
        onSubmit: async reason => { const result = await api("users/reset", { id, reason }); resetCodeDialog(result); onChange?.(); },
      }) }, icon("lock"), "重置密码"),
      h("button", { class: "btn primary", type: "button", onclick: () => {
        const days = h("input", { id: "adjust-days", type: "number", min: -3650, max: 3650, step: 1, value: config.duration_days, required: true });
        userActionDialog({
          title: "调整授权期限", description: "正数延长、负数缩短；从当前到期时间（已到期则从现在）开始计算。",
          fields: [h("div", { class: "field" }, h("label", { for: "adjust-days" }, "天数"), days)], reasonLabel: "调整原因", confirm: "确认调整",
          onSubmit: async reason => {
            const value = Number(days.value);
            if (!Number.isInteger(value) || value === 0 || Math.abs(value) > 3650) throw new Error("天数需在 -3650 至 3650 之间且不为 0");
            await api("users/adjust", { id, days: value, reason });
            toast("授权期限已调整");
            refresh();
          },
        });
      } }, icon("renew"), "调整期限"),
    ],
  });
}

// --------------------------------------------------------------------- Audit

async function renderAudit(view, params, alive) {
  const action = params.get("action") || "";
  const page = Number(params.get("page")) || 1;
  const data = await api("audit" + query({ action, page }));
  if (!alive() || pastEnd(data, "audit", { action })) return;
  const tone = { "login.failed": "bad", "codes.revoke": "bad", "batches.revoke": "bad", "codes.issue": "brand", "devices.transfer": "info", "devices.renew": "info",
    "users.disable": "bad", "users.enable": "ok", "users.adjust": "info", "users.device-remove": "warn", "users.reset": "warn" };
  const rows = data.items.map(entry => h("tr", {},
    h("td", { title: fmtTime(entry.time) }, fmtShort(entry.time)),
    h("td", {}, badge([AUDIT_ACTION[entry.action] || entry.action, tone[entry.action] || ""])),
    h("td", { class: "mono wrap" }, entry.detail || h("span", { class: "muted" }, "—")),
    h("td", { class: "mono muted" }, entry.address || "—")));
  view.replaceChildren(
    pageHead("操作日志", "记录登录与所有写操作，不包含激活码原文。保存在授权服务状态目录的 admin-audit.jsonl。"),
    h("div", { class: "toolbar" }, chips([["", "全部"], ["login", "登录"], ["users", "用户"], ["codes", "激活码"], ["batches", "批次"], ["devices", "设备"]], action, value => navigate("audit", { action: value }), "操作类型")),
    h("section", { class: "card" }, data.items.length
      ? [table(["时间", "操作", "详情", "来源地址"], rows), pager(data, next => navigate("audit", { action, page: next }))]
      : emptyState("audit", "暂无记录", "管理员操作会记录在这里。")),
  );
}

// ---------------------------------------------------------------------- Boot

async function boot() {
  try {
    const current = await api("session");
    if (!current.authenticated) {
      renderLogin();
      return;
    }
    session = current;
    config = { ...config, ...current };
    renderShell();
  } catch (error) {
    app.replaceChildren(h("main", { class: "login-solo" }, h("div", { class: "login-card" },
      h("h1", {}, "无法连接授权服务"), h("p", {}, error.message),
      h("button", { class: "btn primary", type: "button", onclick: boot }, "重试"))));
  }
}

window.addEventListener("hashchange", route);
boot();
