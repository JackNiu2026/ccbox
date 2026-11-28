import { useState } from "react";
import { api } from "./api";
import { translate as tr } from "./message-catalog.mjs";
import { SafetyMessage } from "./safety-message";

const resultLabels = { ok: "可用", connection: "连接失败", response: "返回错误", no_address: "未返回可用地址" };

export function InitializationDNS({ applied, busy, onSave }) {
  const [draft, setDraft] = useState(null);
  const [pending, setPending] = useState("");
  const [result, setResult] = useState(null);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const disabled = busy || Boolean(pending);

  async function load() {
    if (draft || pending) return;
    setPending("load");
    setError("");
    try {
      const settings = await api("/settings");
      setDraft({ primary: settings.dns_url || "", fallback: (settings.dns_fallback_urls || []).join("\n") });
    } catch (failure) {
      setError(failure.message);
    } finally {
      setPending("");
    }
  }

  function edit(key, value) {
    setDraft(current => ({ ...current, [key]: value }));
    setResult(null);
    setError("");
    setSaved(false);
  }

  async function submit(mode) {
    if (!draft || !applied || disabled) return;
    setPending(mode);
    setError("");
    setSaved(false);
    setResult(null);
    try {
      const settings = await api("/settings");
      const candidate = {
        ...settings,
        dns_url: draft.primary.trim(),
        dns_fallback_urls: [...new Set(draft.fallback.split(/\r?\n/).map(line => line.trim()).filter(Boolean))],
      };
      if (mode === "check") setResult(await api("/settings/dns-check", candidate));
      else setSaved(await onSave(candidate));
    } catch (failure) {
      setError(failure.message);
    } finally {
      setPending("");
    }
  }

  return <details className="initialization-dns" onToggle={event => { if (event.currentTarget.open) void load(); }}>
    <summary>{tr("加密 DNS")}</summary>
    <p className="field-help">{tr("通过已应用的代理测试 Debian 软件源域名解析。")}</p>
    {draft && <>
      <label>{tr("加密 DNS")}<input value={draft.primary} onChange={event => edit("primary", event.target.value)} disabled={disabled} spellCheck={false} /></label>
      <label>{tr("备用加密 DNS（每行一个，最多 4 个）")}<textarea rows={2} value={draft.fallback} onChange={event => edit("fallback", event.target.value)} disabled={disabled} spellCheck={false} /></label>
      {!applied && <p className="field-help">{tr("请先选择并应用代理节点，再测试加密 DNS。")}</p>}
      <div className="initialization-recovery">
        <button type="button" disabled={!applied || disabled} onClick={() => submit("check")}>{tr(pending === "check" ? "正在测试加密 DNS…" : "测试加密 DNS")}</button>
        <button type="button" disabled={!applied || disabled} onClick={() => submit("save")}>{tr("保存设置")}</button>
      </div>
    </>}
    {result && <ul role="status">{result.endpoints.map((endpoint, index) => <li key={endpoint.host + "-" + index}>
      <code>{endpoint.host}</code> · {tr(resultLabels[endpoint.status] || "连接失败")}
    </li>)}</ul>}
    {saved && <p role="status">{tr("已保存加密 DNS，可以继续安装。")}</p>}
    {error && <p role="alert"><SafetyMessage message={error} /></p>}
  </details>;
}
