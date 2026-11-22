import React, { useRef, useState } from "react";
import {
  ShieldCheck,
  ShieldAlert,
  Check,
  CircleHelp,
  X,
  LoaderCircle,
  ChevronDown,
} from "lucide-react";
import { api } from "./api";
import { Modal } from "./modal";
import { networkStartAction, networkState } from "./safety-state.mjs";

export async function bindDetectedMacOSExit(ip, onConfirm) {
  if (!(await onConfirm({ title: "绑定固定出口？", body: `客机终端、浏览器和代理均测得 ${ip}。保存后请重启客机并重新检测。`, confirm: "绑定固定出口" }))) return null;
  return api("/macos/exit-policy", { ip });
}

export function SafetyPanel({
  status,
  onClose,
  onConfirm,
  onChanged,
}) {
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const requestInFlight = useRef(false);
  const network = networkState(status),
    safety = status.safety || {},
    report = safety.report;
  const jobRunning = Boolean(status.job?.running),
    busy = pending || jobRunning,
    online = network.mode === "open";
  const passed =
    report?.checks?.filter((c) => c.critical && c.state === "passed").length ||
    0;
  const total = report?.checks?.filter((c) => c.critical).length || 0;
  const submit = async (path, confirmRestart = false) => {
    if (requestInFlight.current) return;
    requestInFlight.current = true;
    setPending(true);
    setError("");
    try {
      if (confirmRestart && !(await onConfirm({
        title: "重启环境并检查外网？",
        body: "当前终端会话将结束。请先保存文件，浏览器登录与项目文件会保留。",
        confirm: "重启并检查",
      }))) return;
      await api(path, {});
      await onChanged?.();
    } catch (e) {
      setError(e.message);
    } finally {
      requestInFlight.current = false;
      setPending(false);
    }
  };
  const run = () => {
    const { path, confirmRestart } = networkStartAction(status);
    return submit(path, confirmRestart);
  };
  return (
    <Modal
      title="网络与隔离检查"
      wide
      onClose={onClose}
      footer={
        <>
          <span className="save-note" role="status">
            {jobRunning ? status.job.message : pending ? "正在提交请求…" : "外网关闭时，离线终端和文件仍可使用"}
          </span>
          <button disabled={!status.vm_running || pending} onClick={() => submit("/safety/close")}>关闭外网</button>
          <button className="primary" disabled={busy} onClick={run}>
            {busy ? (
              <LoaderCircle size={15} className="spin" />
            ) : (
              <ShieldCheck size={15} />
            )}
            检测并启动
          </button>
        </>
      }
    >
      <div className="modal-body safety-body">
        <div className={`safety-summary ${network.mode}`}>
          {online ? (
            <ShieldCheck size={27} />
          ) : network.mode === "checking" ? (
            <LoaderCircle size={27} className="spin" />
          ) : (
            <ShieldAlert size={27} />
          )}
          <div>
            <h3>{network.label}</h3>
            <p>
              {jobRunning
                ? status.job.message
                : online
                  ? status.network_error || "关键检查已通过，后台继续核对出口规则与环境配置。"
                  : status.network_error || safety.error ||
                    safety.gate?.reason ||
                    "完成关键检查后，才允许连接外部网站。"}
            </p>
          </div>
        </div>
        {error && (
          <div role="alert" className="error-message">{error}</div>
        )}
        <div className="safety-section-heading">
          <h4>关键检查</h4>
          <span>{total ? `${passed} / ${total} 通过` : "尚无检测记录"}</span>
        </div>
        {report ? (
          <>
            <p className="safety-timestamp">
              检测于 {new Date(report.created_at).toLocaleString("zh-CN")} ·{" "}
              {online ? "当前运行实例" : "报告不代表当前已放行"}
            </p>
            <div className="safety-checks">
              {report.checks
                .filter((check) => check.critical)
                .map((check) => {
                  const Icon =
                    check.state === "passed"
                      ? Check
                      : check.state === "failed"
                        ? X
                        : CircleHelp;
                  return (
                    <details
                      className={`safety-check ${check.state}`}
                      key={check.id}
                    >
                      <summary>
                        <Icon size={16} />
                        <strong>{check.title}</strong>
                        <span>
                          {check.state === "passed"
                            ? "通过"
                            : check.state === "failed"
                              ? "未通过"
                              : jobRunning && check.checked_at === report.created_at
                                ? "待检查"
                                : "未知"}
                        </span>
                        <ChevronDown size={14} />
                      </summary>
                      <div>
                        <p>{check.evidence}</p>
                        {check.state !== "passed" && check.remedy && (
                          <p className="check-remedy">{check.remedy}</p>
                        )}
                      </div>
                    </details>
                  );
                })}
            </div>
          </>
        ) : (
          <p className="safety-empty">
            检查涵盖出站规则、代理故障关闭、DNS、共享目录、宿主管理接口，以及系统和浏览器的实际配置。关键项失败或未知时保持外网关闭。
          </p>
        )}
        {report?.checks?.some((check) => !check.critical) && (
          <div className="safety-advisories">
            {report.checks
              .filter((check) => !check.critical)
              .map((check) => (
                <p key={check.id}>
                  <CircleHelp size={15} />
                  <span>
                    <strong>{check.title}：</strong>
                    {check.evidence}
                  </span>
                </p>
              ))}
          </div>
        )}
        <details className="safety-scope">
          <summary>检查范围</summary>
          <p>
            当前版本验证 IPv4 出口，客机公网 IPv6 与 UDP 关闭。出口监测约每 30
            秒执行；位置来自 IP
            数据库，城市判断具有误差。代理自身按目标分流的行为仍需在代理侧约束。
          </p>
          <p>
            报告包含运行时策略和采样证据，不能代替完整虚拟化审计，也不代表平台账号资格或封禁预测。全面检查有效期为
            24 小时；控制服务失联后，联网授权最多保留 90 秒。
          </p>
        </details>
      </div>
    </Modal>
  );
}
