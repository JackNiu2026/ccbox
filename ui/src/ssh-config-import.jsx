import React, { useEffect, useState } from "react";
import { api } from "./api";
import { Modal } from "./modal";

export function SSHConfigImport({ platform, onSaved, onClose }) {
  const configPath = platform === "windows" ? "%USERPROFILE%\\.ssh\\config" : "~/.ssh/config";
  const [preview, setPreview] = useState(null);
  const [selected, setSelected] = useState(new Set());
  const [reload, setReload] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState(null);

  useEffect(() => {
    const controller = new AbortController();
    setPreview(null);
    setSelected(new Set());
    setError("");
    api("/client/ssh-config", undefined, "GET", {}, { signal: controller.signal }).then(
      (value) => { if (!controller.signal.aborted) setPreview(value); },
      (failure) => { if (!controller.signal.aborted) setError(failure.message); },
    );
    return () => controller.abort();
  }, [reload]);

  const save = async () => {
    setBusy(true);
    setError("");
    try {
      const value = await api("/client/ssh-config", { revision: preview.revision, aliases: [...selected] });
      setResult(value);
      await onSaved(value.imported);
    } catch (failure) {
      setError(failure.message);
    } finally {
      setBusy(false);
    }
  };

  return <Modal title="导入本机 SSH 配置" onClose={busy ? undefined : onClose}>
    <div className="ssh-dialog-body">
      <p>从 {configPath} 读取主机。连接时使用本机 SSH agent，请先在 agent 中添加密钥。</p>
      <p>已有主机继续保留，导入的主机首次连接时需要核对指纹。</p>
      {error && <p className="error-message" role="alert">{error}</p>}
      {result ? <p role="status">已导入 {result.imported.length} 台主机，跳过 {result.skipped} 台已有主机。</p> : <>
        {!preview && !error && <p role="status">正在读取 SSH 配置…</p>}
        {preview?.hosts.length === 0 && <p>没有可导入的主机。</p>}
        <div className="ssh-config-hosts">
          {preview?.hosts.map((host) => <label key={host.name}>
            <input type="checkbox" checked={selected.has(host.name)} disabled={busy}
              onChange={(event) => {
                const checked = event.target.checked;
                setSelected((old) => {
                  const next = new Set(old);
                  if (checked) next.add(host.name);
                  else next.delete(host.name);
                  return next;
                });
              }} />
            <span><strong>{host.name}</strong><small>{host.user}@{host.host}:{host.port}</small></span>
          </label>)}
        </div>
        {Boolean(preview?.skipped.length) && <details>
          <summary>已跳过的主机（{preview.skipped.length}）</summary>
          <ul>{preview.skipped.map((host) => <li key={host.name}>{host.name} · {host.reason}</li>)}</ul>
        </details>}
      </>}
      <div className="remote-button-row">
        {result ? <button className="primary" disabled={busy} onClick={onClose}>完成并关闭</button> : <>
          <button className="primary" disabled={busy || !preview || !selected.size} onClick={save}>
            {busy ? "正在导入…" : `导入所选主机（${selected.size}）`}
          </button>
          <button disabled={busy} onClick={() => setReload((old) => old + 1)}>重新读取</button>
          <button disabled={busy} onClick={onClose}>取消</button>
        </>}
      </div>
    </div>
  </Modal>;
}
