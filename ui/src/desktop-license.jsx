import React, { useEffect, useState } from "react";
import { api } from "./api";
import "./desktop-license.css";

const labels = {
  active: "此设备已激活",
  expired: "授权已到期",
  inactive: "此设备尚未激活",
  invalid: "授权无法验证",
};

export function useDesktopLicense(enabled = true) {
  const [license, setLicense] = useState(null);
  const [error, setError] = useState("");
  const [attempt, retry] = useState(0);
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    setError("");
    api("/desktop-license", undefined, "GET", {}, { signal: controller.signal })
      .then(setLicense)
      .catch(error => {
        if (!controller.signal.aborted) setError(error.message);
      });
    return () => controller.abort();
  }, [enabled, attempt]);
  return { license, error, retry: () => retry(attempt + 1), setLicense };
}

export function DesktopLicenseForm({ license, onActivated }) {
  const [code, setCode] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const active = license.state === "active";
  const expiry = license.expires ? new Intl.DateTimeFormat(undefined, {
    dateStyle: "long", timeStyle: "short",
  }).format(new Date(license.expires * 1000)) : "";

  async function activate(event) {
    event.preventDefault();
    if (pending || !code.trim()) return;
    setPending(true);
    setError("");
    setSuccess("");
    try {
      const updated = await api("/desktop-license", { code: code.trim() });
      setCode("");
      setSuccess("授权已更新。");
      onActivated(updated);
    } catch (error) {
      setError(error.message);
    } finally {
      setPending(false);
    }
  }

  return <form className="desktop-license-form" onSubmit={activate}>
    <div className="desktop-license-state">
      <strong>{labels[license.state]}</strong>
      {expiry && <p>有效期至 {expiry}</p>}
      <p>{active ? "提前续期会在当前到期时间后增加 30 天。" : "输入激活码，为此设备开通 30 天。"}</p>
    </div>
    <label htmlFor="desktop-license-code">激活码</label>
    <input id="desktop-license-code" type="text" value={code} maxLength={64} spellCheck={false}
      autoComplete="off" autoCapitalize="characters" disabled={pending}
      onChange={event => setCode(event.target.value)} />
    {error && <p role="alert">{error}</p>}
    {success && <p role="status">{success}</p>}
    <button className="primary" type="submit" disabled={pending || !code.trim()}>
      {pending ? "正在验证…" : active ? "续期 30 天" : "激活此设备"}
    </button>
    <details><summary>换机所需的设备编号</summary><code>{license.device}</code></details>
  </form>;
}

export function DesktopLicenseSettings() {
  const { license, error, retry, setLicense } = useDesktopLicense();
  return <section aria-label="授权">
    <h3>授权</h3>
    {error ? <><p role="alert">{error}</p><button onClick={retry}>重试</button></>
      : !license ? <p role="status">正在读取授权状态…</p>
      : license.state === "disabled" ? <p>当前构建无需激活。</p>
      : <DesktopLicenseForm license={license} onActivated={setLicense} />}
  </section>;
}
