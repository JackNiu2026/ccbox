import React, { useEffect, useRef, useState } from "react";
import { api, native, nativeAvailable } from "./api";
import { getClientContext, runtimeURL } from "./client-context.mjs";
import { InlineSelect } from "./inline-select";
import {
  createNotificationDeduplicator,
  defaultNotificationPreferences,
  dispatchDesktopNotification,
  normalizeNotificationPreferences,
  requestNativeNotificationPermission,
  testNativeNotification,
} from "./notifications.mjs";

const preferencesChanged = "cage-notification-preferences";
const notificationClicked = "cage-notification-click";
const enabled = value => value.turn_completion !== "off" || value.approval !== "off";

export function useTerminalNotifications({ ready, connection, guestSystem, active, other, overlay, onSelect }) {
  const [preferences, setPreferences] = useState(defaultNotificationPreferences);
  const dedup = useRef(null);
  if (!dedup.current) dedup.current = createNotificationDeduplicator();
  const view = useRef(null);
  view.current = { preferences, active, other, overlay, onSelect };

  useEffect(() => {
    const controller = new AbortController();
    let changed = false;
    const update = event => {
      changed = true;
      setPreferences(normalizeNotificationPreferences(event.detail));
    };
    const select = event => {
      const { terminal_id, connection_id } = event.detail || {};
      if (terminal_id && connection_id === getClientContext().connection_id) {
        view.current.onSelect("terminal:" + terminal_id);
      }
    };
    window.addEventListener(preferencesChanged, update);
    window.addEventListener(notificationClicked, select);
    api("/client/notifications", undefined, undefined, {}, { signal: controller.signal })
      .then(value => {
        if (!controller.signal.aborted && !changed) setPreferences(normalizeNotificationPreferences(value));
      }).catch(() => {});
    return () => {
      controller.abort();
      window.removeEventListener(preferencesChanged, update);
      window.removeEventListener(notificationClicked, select);
    };
  }, []);

  const listening = enabled(preferences);
  useEffect(() => {
    const context = getClientContext();
    if (!ready || !listening || !nativeAvailable() || context.mode !== "local" || context.client_only || /^(windows|macos)/.test(guestSystem)) return;
    const stream = new EventSource(runtimeURL("/notifications/events", context));
    stream.addEventListener("notification", event => {
      if (getClientContext().connection_id !== connection) return;
      let detail;
      try { detail = JSON.parse(event.data); }
      catch { return; }
      const current = view.current;
      const windowFocused = document.visibilityState !== "hidden" &&
        window.cageWindowVisible !== false && (window.cageWindowFocused ?? document.hasFocus());
      const viewingTerminal = !current.overlay && Boolean(detail.tab_id && [current.active, current.other].includes(detail.tab_id));
      void dispatchDesktopNotification(detail, {
        prefs: current.preferences,
        viewState: { windowFocused, viewingTerminal },
        nativeAvailable,
        native,
        connectionId: connection,
        dedup: dedup.current,
      }).catch(() => {});
    });
    return () => stream.close();
  }, [ready, listening, connection, guestSystem]);
}

const modes = [
  ["off", "关闭"],
  ["unfocused", "未查看该终端时"],
  ["always", "始终通知"],
];

export function TerminalNotificationSettings() {
  const [preferences, setPreferences] = useState(null);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [permission, setPermission] = useState(null);
  useEffect(() => {
    const controller = new AbortController();
    api("/client/notifications", undefined, undefined, {}, { signal: controller.signal })
      .then(value => setPreferences(normalizeNotificationPreferences(value)))
      .catch(error => { if (!controller.signal.aborted) setMessage(error.message); });
    const refreshPermission = () => {
      if (nativeAvailable()) void native("notificationStatus")
        .then(result => { if (!controller.signal.aborted) setPermission(result); })
        .catch(() => {});
    };
    refreshPermission();
    window.addEventListener("focus", refreshPermission);
    return () => {
      controller.abort();
      window.removeEventListener("focus", refreshPermission);
    };
  }, []);

  const showPermission = status => {
    setPermission(status);
    if (status?.supported && ["authorized", "provisional"].includes(status.permission)) return true;
    setMessage(status?.supported ? "CCBox 通知尚未获得系统授权。请在系统设置中允许通知。" : "此设备当前无法发送系统通知。");
    return false;
  };

  const save = async (key, value) => {
    if (saving || !preferences) return;
    setSaving(true);
    setMessage("");
    try {
      const saved = normalizeNotificationPreferences(await api("/client/notifications", { ...preferences, [key]: value }));
      setPreferences(saved);
      window.dispatchEvent(new CustomEvent(preferencesChanged, { detail: saved }));
      if (key !== "sound" && value !== "off" && nativeAvailable()) {
        showPermission(await requestNativeNotificationPermission(native));
      }
    } catch (error) { setMessage(error.message); }
    finally { setSaving(false); }
  };

  const testNotification = async () => {
    if (saving || !preferences) return;
    setSaving(true);
    setMessage("");
    try {
      const result = await testNativeNotification(native, preferences.sound);
      if (showPermission(result.status)) {
        setMessage(result.delivered ? "已发送测试通知。" : "系统未显示通知，请检查 CCBox 的系统通知权限。");
      }
    } catch (error) { setMessage(error.message); }
    finally { setSaving(false); }
  };

  const openSettings = async () => {
    try { await native("openNotificationSettings"); }
    catch (error) { setMessage(error.message); }
  };

  return <section className="settings-fields">
    <h3>Claude Code 终端通知</h3>
    <p className="field-help">通知仅显示在运行本机环境的电脑上。通知内容包含项目和终端名称，不包含提示词、回复或待审批内容。</p>
    {preferences && <>
      {[["turn_completion", "回合完成"], ["approval", "等待确认"]].map(([key, label]) => <label key={key}>
        {label}
        <InlineSelect aria-label={label} value={preferences[key]} disabled={saving} onValueChange={value => void save(key, value)}>
          {modes.map(([mode, name]) => <option key={mode} value={mode}>{name}</option>)}
        </InlineSelect>
      </label>)}
      {permission?.sound && <label><input type="checkbox" checked={preferences.sound} disabled={saving} onChange={event => void save("sound", event.target.checked)} /> 播放提示音</label>}
      <button type="button" className="outline" disabled={saving || !nativeAvailable()} onClick={testNotification}>发送测试通知</button>
      {permission?.canOpenSettings && permission.permission === "denied" &&
        <button type="button" className="outline" onClick={openSettings}>打开系统通知设置</button>}
    </>}
    {message && <p role="status" className="field-help">{message}</p>}
  </section>;
}
