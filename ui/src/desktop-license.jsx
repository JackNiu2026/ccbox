import React, { useEffect, useState } from "react";
import { api } from "./api";
import { translate as tr } from "./message-catalog.mjs";
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
  // A signed-in account shows its own term; the device grant inside it is
  // shorter and renews in the background.
  const shown = license.account && license.account_expires ? license.account_expires : license.expires;
  const expiry = shown ? new Intl.DateTimeFormat(undefined, {
    dateStyle: "long", timeStyle: "short",
  }).format(new Date(shown * 1000)) : "";

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
      <strong>{tr(labels[license.state])}</strong>
      {expiry && <p>{tr("有效期至 {0}", expiry)}</p>}
      <p>{license.account
        ? tr("激活码兑换到线上账户 {0}，{1}；该账户登录的其他电脑同样可用。", license.account, tr(active ? "在当前到期时间后增加 30 天" : "开通 30 天"))
        : tr(active ? "提前续期会在当前到期时间后增加 30 天。" : "输入激活码，为此设备开通 30 天。")}</p>
    </div>
    <label htmlFor="desktop-license-code">{tr("激活码")}</label>
    <input id="desktop-license-code" type="text" value={code} maxLength={64} spellCheck={false}
      autoComplete="off" autoCapitalize="characters" disabled={pending}
      onChange={event => setCode(event.target.value)} />
    {error && <p role="alert">{error}</p>}
    {success && <p role="status">{tr(success)}</p>}
    <button className="primary" type="submit" disabled={pending || !code.trim()}>
      {tr(pending ? "正在验证…" : active ? "续期 30 天" : "激活此设备")}
    </button>
    <details><summary>{tr("换机所需的设备编号")}</summary><code>{license.device}</code></details>
  </form>;
}

export function DesktopLicenseSettings() {
  const { license, error, retry, setLicense } = useDesktopLicense();
  return <section aria-label={tr("授权")}>
    <h3>{tr("授权")}</h3>
    {error ? <><p role="alert">{error}</p><button onClick={retry}>{tr("重试")}</button></>
      : !license ? <p role="status">{tr("正在读取授权状态…")}</p>
      : license.state === "disabled" ? <p>{tr("当前构建无需激活。")}</p>
      : <DesktopLicenseForm license={license} onActivated={setLicense} />}
  </section>;
}
