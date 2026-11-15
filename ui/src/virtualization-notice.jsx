import React, { useEffect, useState } from "react";
import { api } from "./api";
import { actionConfirmation, actionDone, actionProgress, virtualizationActions } from "./virtualization-state.mjs";
import "./virtualization-notice.css";

// Shown under a Linux guest choice: checks the host before any image is
// downloaded and offers the fixes the server reports as possible.
export function VirtualizationNotice({ onReady }) {
  const [report, setReport] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [pendingConfirm, setPendingConfirm] = useState("");
  const [message, setMessage] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    setReport(null);
    setError("");
    api("/environment/virtualization").then((next) => {
      if (!active) return;
      setReport(next);
      onReady(Boolean(next.ready));
    }).catch((failure) => {
      if (!active) return;
      // Remote or older backends have no check; the install path still has one.
      if (failure.status === 403 || failure.status === 404) onReady(true);
      else setError(failure.message);
    });
    return () => { active = false; };
  }, [attempt, onReady]);

  async function run(action) {
    setPendingConfirm("");
    setBusy(action);
    setError("");
    setMessage("");
    try {
      const result = await api("/environment/virtualization", { action });
      if (action === "enable") {
        setReport(result);
        onReady(Boolean(result.ready));
        setMessage(result.ready ? "虚拟化已就绪，可以继续安装。" : "设置已完成，重启电脑后生效。");
      } else {
        setMessage(actionDone[action]);
      }
    } catch (failure) {
      setError(failure.message);
    } finally {
      setBusy("");
    }
  }

  function recheck() {
    setMessage("");
    setPendingConfirm("");
    setAttempt((value) => value + 1);
  }

  if (error && !report) {
    return <div className="guest-install-fields virtualization-notice" role="alert">
      <small>无法检查这台电脑的虚拟化：{error}</small>
      <div><button type="button" onClick={recheck}>重新检测</button></div>
    </div>;
  }
  if (!report) return <small className="guest-install-fields" role="status">正在检查这台电脑的虚拟化…</small>;
  if (report.ready) return <small className="guest-install-fields virtualization-ready" role="status">✓ 这台电脑的虚拟化已就绪</small>;

  const firmware = report.firmware;
  const actions = virtualizationActions(report);
  return <div className="guest-install-fields virtualization-notice" role="alert" aria-live="polite">
    <strong>{report.summary}</strong>
    {report.steps?.length > 0 && <ol>{report.steps.map((step) => <li key={step}>{step}</li>)}</ol>}
    {firmware && <div className="virtualization-firmware">
      <small><b>{firmware.vendor ? `${firmware.vendor}电脑进入 BIOS：` : "进入 BIOS："}</b>{firmware.keys}</small>
      <small><b>要开启的选项：</b>{firmware.option}</small>
      {firmware.location && <small><b>常见位置：</b>{firmware.location}</small>}
      {firmware.windows?.map((path) => <small key={path}><b>不确定按键时：</b>{path}</small>)}
    </div>}
    {report.detail && <small className="virtualization-detail">{report.detail}</small>}
    {pendingConfirm ? <div className="virtualization-confirm">
      <small>{actionConfirmation[pendingConfirm]}</small>
      <div>
        <button type="button" className="primary" onClick={() => run(pendingConfirm)}>确定重启</button>
        <button type="button" onClick={() => setPendingConfirm("")}>取消</button>
      </div>
    </div> : <div>
      {actions.map(({ action, label, primary }) => <button key={action} type="button" className={primary ? "primary" : undefined}
        disabled={Boolean(busy)} onClick={() => actionConfirmation[action] ? setPendingConfirm(action) : run(action)}>
        {busy === action ? actionProgress[action] : label}
      </button>)}
      <button type="button" disabled={Boolean(busy)} onClick={recheck}>重新检测</button>
    </div>}
    {message && <small role="status" className="virtualization-message">{message}</small>}
    {error && <small role="alert" className="virtualization-error">{error}</small>}
  </div>;
}
