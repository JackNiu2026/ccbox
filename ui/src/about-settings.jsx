import React, { useEffect, useState } from "react";
import { api } from "./api";
import "./about-settings.css";

export function AboutSettings() {
  const [build, setBuild] = useState(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    api("/client/about", undefined, undefined, {}, { signal: controller.signal })
      .then(setBuild)
      .catch(error => { if (!controller.signal.aborted) setError(error.message); });
    return () => controller.abort();
  }, []);
  const pending = !build && !error;
  return <section className="about-settings" aria-label="关于 CCBox">
    <div className="section-title"><h3>关于</h3><p>CCBox 的版本与构建信息</p></div>
    <dl className="about-build">
      <div><dt>软件版本</dt><dd>{pending ? "正在读取…" : build?.version || "未知"}</dd></div>
      <div><dt>构建提交</dt><dd>{build?.commit ? <code title={build.commit}>{build.commit.slice(0, 12)}</code> : pending ? "正在读取…" : "未记录"}</dd></div>
    </dl>
    {build?.modified && <p className="footnote">此构建包含未提交的修改。</p>}
    {error && <p className="error-message" role="alert">{error}</p>}
  </section>;
}
