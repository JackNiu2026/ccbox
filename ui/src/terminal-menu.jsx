import React, { useLayoutEffect, useRef, useState } from "react";
import { TerminalSquare, UserRound } from "lucide-react";
import { AgentIcon } from "./agent-icon";
import { InlineSelect } from "./inline-select";
import { translate as tr } from "./message-catalog.mjs";
import { terminalAccountSelection } from "./terminal-menu-state.mjs";

export function TerminalMenu({ choice, busy, profiles, onClose, onSelect }) {
  const menu = useRef(null);
  const [configuring, setConfiguring] = useState(false);
  const [preferred, setPreferred] = useState("");
  const [handoff, setHandoff] = useState();
  const [loading, setLoading] = useState(false);
  const close = useRef(onClose);
  close.current = onClose;
  useLayoutEffect(() => {
    const node = menu.current;
    const anchor = choice.anchor;
    if (!anchor?.isConnected) {
      close.current();
      return;
    }
    anchor.setAttribute("aria-expanded", "true");
    anchor.setAttribute("aria-controls", "terminal-create-menu");
    node.querySelector("button:not(:disabled)")?.focus();
    const dismiss = (restoreFocus = false) => {
      if (restoreFocus) anchor.focus();
      close.current();
    };
    const outside = (event) => {
      if (!node.contains(event.target) && !anchor.contains(event.target)) dismiss();
    };
    const keydown = (event) => {
      if (event.defaultPrevented) return;
      if (event.key === "Escape" || (!configuring && event.key === "Tab")) {
        if (event.key === "Escape") event.preventDefault();
        event.stopPropagation();
        dismiss(true);
      } else if (!configuring && ["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
        event.preventDefault();
        const items = [...node.querySelectorAll("button:not(:disabled)")];
        const index = items.indexOf(document.activeElement);
        const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1
          : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
        items[next]?.focus();
      }
    };
    const closeCommand = () => dismiss(true);
    document.addEventListener("click", outside, true);
    document.addEventListener("keydown", keydown, !configuring);
    document.addEventListener("cage-close-modal", closeCommand);
    return () => {
      anchor.setAttribute("aria-expanded", "false");
      anchor.removeAttribute("aria-controls");
      document.removeEventListener("click", outside, true);
      document.removeEventListener("keydown", keydown, !configuring);
      document.removeEventListener("cage-close-modal", closeCommand);
    };
  }, [choice, configuring]);
  useLayoutEffect(() => {
    if (configuring && !loading && (document.activeElement === document.body || !menu.current?.contains(document.activeElement))) {
      menu.current?.querySelector('[role="combobox"]:not(:disabled), button:not(:disabled)')?.focus();
    }
  }, [configuring, loading]);

  const configure = async () => {
    setConfiguring(true);
    setLoading(true);
    try { await profiles.load(); }
    finally { setLoading(false); }
  };
  if (configuring && profiles) {
    const { account, autoAvailable, readyAccounts, hasBackup, autoHandoff } =
      terminalAccountSelection(profiles.accounts, preferred, profiles.scheduling?.switch_policy, handoff);
    const label = tr("启动账号");
    return <form ref={menu} id="terminal-create-menu" className="menu terminal-create-menu terminal-account-form"
      aria-label={tr("选择启动账号")} onSubmit={event => {
        event.preventDefault();
        if (busy || loading || profiles.error) return;
        if (account === "login") onSelect("claude-login");
        else onSelect("claude", account === "auto" ? undefined : account, autoHandoff);
      }}>
      <label htmlFor="terminal-account-choice">{label}</label>
      <InlineSelect inline id="terminal-account-choice" aria-label={label}
        value={account} disabled={busy || loading} onValueChange={value => { setPreferred(value); setHandoff(undefined); }}>
        <option value="auto" disabled={!autoAvailable}>{tr(profiles.accounts?.length ? "自动选择账号" : "使用环境默认登录")}</option>
        {readyAccounts.map(profile => <option key={profile.id} value={profile.id}>
          {profile.number ? "#" + String(profile.number).padStart(2, "0") + " " + profile.label : profile.label}
        </option>)}
        <option value="login">{tr("在此终端登录")}</option>
      </InlineSelect>
      {hasBackup && <label className="check-line">
        <input type="checkbox" checked={autoHandoff} disabled={busy || loading}
          onChange={event => setHandoff(event.target.checked)} />
        {tr("额度用尽后自动接续")}
      </label>}
      {loading && <small role="status">{tr("正在读取 Claude 账号…")}</small>}
      {!loading && profiles.error && <small role="alert">{tr("暂时无法刷新 Claude 账号。")}</small>}
      {profiles.error
        ? <button type="button" disabled={busy || loading} onClick={configure}>{tr("重试")}</button>
        : <button type="submit" disabled={busy || loading}>{tr("创建 Claude 终端")}</button>}
    </form>;
  }

  return (
    <div ref={menu} id="terminal-create-menu" className="menu terminal-create-menu" role="menu" aria-label="新建终端">
      <button role="menuitem" disabled={busy} onClick={() => onSelect("shell")}>
        <TerminalSquare size={16} />普通终端
      </button>
      <button role="menuitem" disabled={busy} onClick={() => onSelect("claude")}>
        <AgentIcon />Claude 终端
      </button>
      {profiles && <button role="menuitem" disabled={busy} onClick={configure}>
        <UserRound size={16} />{tr("选择启动账号")}
      </button>}
    </div>
  );
}
