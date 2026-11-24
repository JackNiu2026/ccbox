import { translate as tr } from "./message-catalog.mjs";
import React, { useEffect, useState } from "react";
import { api } from "./api";

const endpoint = "/environment/device-reset";

export function EnvironmentDeviceReset({ status, onConfirm }) {
  const [info, setInfo] = useState(null);
  const [selected, setSelected] = useState([]);
  const [requesting, setRequesting] = useState(false);
  const [jobId, setJobId] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const pending = requesting || Boolean(jobId) || status.job?.running;

  async function inspect() {
    setRequesting(true);
    setError("");
    try {
      const next = await api(endpoint);
      setInfo(next);
      setSelected([]);
    } catch (failure) {
      setError(failure.message);
    } finally {
      setRequesting(false);
    }
  }

  useEffect(() => {
    const job = status.job;
    if (!jobId || job?.id !== jobId || job.running) return;
    setJobId("");
    if (job.error || job.canceled) setError(job.error || "操作已取消");
    else setMessage("操作完成。可重新检查环境与归档状态。");
    setInfo(null);
  }, [jobId, status.job]);

  async function confirmOperation(action, payload, title, body) {
    if (!(await onConfirm({ title, body, confirm: title }))) return;
    setRequesting(true);
    setError("");
    setMessage("");
    try {
      const result = await api(`${endpoint}/${action}`, payload);
      if (result.id) setJobId(result.id);
      else {
        setMessage("操作完成。可重新检查环境与归档状态。");
        setInfo(null);
      }
    } catch (failure) {
      setError(failure.message);
    } finally {
      setRequesting(false);
    }
  }

  const restore = (kind, id) => confirmOperation("restore", { kind, id }, "恢复旧环境资料",
    kind === "archive" ? "会先停止当前环境，并为当前磁盘保留可恢复归档，再切回选择的旧环境。" : "会在当前环境恢复所选批次。遇到同名数据会保留两边。");

  return <details>
    <summary>环境换机与恢复</summary>
    <p className="field-help">普通升级会继续使用原环境。只有在这里主动确认，才会轮换设备标识或创建新环境。</p>
    <button type="button" disabled={pending} onClick={inspect}>检查环境与归档</button>
    {error && <p role="alert">{error}</p>}
    {message && <p role="status">{message}</p>}
    {jobId && <p role="status">{status.job?.message || "正在处理环境…"}</p>}
    {info && !info.supported && <p role="status">{info.unsupported_reason}</p>}
    {info?.supported && <div className="settings-group">
      {info.targets.filter((target) => target.present).map((target) => <label key={target.id}>
        <input type="checkbox" disabled={pending} checked={selected.includes(target.id)} onChange={(event) => setSelected((items) =>
          event.target.checked ? [...items, target.id] : items.filter((id) => id !== target.id))} />{target.title}
        <small>{target.description}</small>
      </label>)}
      <button type="button" disabled={pending || !info.ready} onClick={() => confirmOperation("execute", { mode: "quick", items: selected }, "快速换机", "会停止并重启当前环境，轮换设备标识，并把已选资料移入可恢复批次。")}>快速换机</button>
      <button type="button" disabled={pending} onClick={() => confirmOperation("execute", { mode: "rebuild", items: [] }, "创建全新环境", "会停止当前环境并保留旧环境归档，再验证全新的 Debian 13 环境。只能在旧客机里看到的项目和登录资料仍保留在旧归档中。")}>创建全新环境</button>
      {info.archives?.filter((archive) => archive.state === "ready").map((archive) => <div key={archive.id} className="field-row">
        <span>{archive.previous_hostname || "旧环境"} · {new Date(archive.created_at * 1000).toLocaleString("zh-CN")}</span>
        <button type="button" disabled={pending} onClick={() => restore("archive", archive.id)}>{tr("恢复旧环境")}</button>
      </div>)}
      {info.recovery_batches?.map((batch) => <div key={batch.id} className="field-row">
        <span>{batch.count} 项资料 · {new Date(batch.created_at * 1000).toLocaleString("zh-CN")}</span>
        <button type="button" disabled={pending || !info.ready} onClick={() => restore("batch", batch.id)}>恢复资料</button>
      </div>)}
    </div>}
  </details>;
}
