import React, {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import {
  Camera,
  Check,
  Eye,
  EyeOff,
  Fingerprint,
  Globe2,
  LockKeyhole,
  LogOut,
  ShieldCheck,
} from "lucide-react";
import { api, native, nativeAvailable } from "./api";
import { getClientContext, isRemoteClient } from "./client-context.mjs";
import logo from "./logo.svg";
import "./account.css";

const AccountContext = createContext(null);
export const useAccount = () => useContext(AccountContext);
const localDesktop = () => getClientContext().desktop_gateway && !isRemoteClient();
const trustedLogin = (action, password) => api("/client/trusted-login", {
  action, password, connection_id: getClientContext().connection_id,
});
const credentialPresent = (status) => status === "available" || status === "authentication_required";

export function AccountProvider({ children }) {
  const [account, setAccount] = useState(null);
  const [error, setError] = useState("");
  const [deviceCheck, setDeviceCheck] = useState(null);
  const deviceKey = JSON.stringify([account?.device_binding, account?.devices?.automatic, account?.devices?.biometric]);
  const deviceSupport = deviceCheck?.key === deviceKey ? deviceCheck.value : null;
  const [deviceNotice, setDeviceNotice] = useState("");
  const sequence = useRef(0);
  const refresh = async () => {
    const current = ++sequence.current;
    try {
      const value = await api("/account");
      if (sequence.current === current) {
        setAccount(value);
        setError("");
      }
      return value;
    } catch (e) {
      if (sequence.current === current) setError(e.message);
      throw e;
    }
  };
  const act = async (action, input = {}) => {
    const current = ++sequence.current;
    const value = await api(`/account/${action}`, input);
    const createsAccount = action === "create" || action === "login";
    if (createsAccount) {
      // A status read started during account creation must not replace the result.
      ++sequence.current;
      setAccount(value);
    } else if (action !== "logout" && sequence.current === current) setAccount(value);
    return value;
  };
  useEffect(() => {
    if (!account || deviceSupport !== null) return;
    if (!nativeAvailable()) { setDeviceCheck({ key: deviceKey, value: { supported: false } }); return; }
    let live = true;
    native("accountDevice", { operation: "status" })
      .then((value) => { if (live) setDeviceCheck({ key: deviceKey, value: value || { supported: false } }); })
      .catch(() => { if (live) setDeviceCheck({ key: deviceKey, value: { supported: false } }); });
    return () => { live = false; };
  }, [Boolean(account), deviceKey, deviceSupport]);
  const deviceAction = async (operation, kind) => {
    if (!deviceSupport?.device_status) throw new Error("当前应用启动器需要更新，请重新安装完整的 CCBox 应用后再设置快捷登录。");
    try {
      await native("accountDevice", { operation, kind });
    } finally {
      // Failed enrollment may have revoked its server record. Reconcile both
      // sides after failures too, including a credential removed while open.
      await refresh();
      setDeviceCheck(null);
    }
  };
  useEffect(() => {
    let live = true,
      timer;
    const poll = async () => {
      try {
        await refresh();
      } catch {
        /* Keep the last state while disconnected. */
      }
      if (live) timer = setTimeout(poll, document.hidden ? 10000 : 2500);
    };
    const start = async () => {
      try {
        if (location.hash) {
          await api("/session", { token: location.hash.slice(1) });
          history.replaceState(null, "", location.pathname);
        }
        window.dispatchEvent(new Event("cage-session-ready"));
        if (live) await poll();
      } catch (e) {
        if (live) setError(e.message);
      }
    };
    const locked = () => {
      // A 423 can follow a controller restart or directory change. Do not
      // auto-unlock using the previous account's cached Keychain status.
      setDeviceCheck(null);
      setAccount((previous) =>
        previous ? { ...previous, unlocked: false } : previous,
      );
      refresh().catch(() => {});
    };
    start();
    window.addEventListener("cage-account-locked", locked);
    return () => {
      live = false;
      sequence.current++;
      clearTimeout(timer);
      window.removeEventListener("cage-account-locked", locked);
    };
  }, []);
  return (
    <AccountContext.Provider value={{ account, error, refresh, act, deviceSupport, deviceAction, deviceNotice, setDeviceNotice, recheckDevices: () => setDeviceCheck(null) }}>
      {children}
    </AccountContext.Provider>
  );
}

export function AccountAvatar({ account, large = false }) {
  const initial = Array.from(
    account?.username?.trim() || "C",
  )[0].toLocaleUpperCase();
  return (
    <span
      className={`account-avatar ${large ? "large" : ""}`}
      aria-hidden="true"
    >
      {account?.avatar ? <img src={account.avatar} alt="" /> : initial}
    </span>
  );
}

export function MacOSAccountGate({ children }) {
  const { account, act } = useAccount();
  const [name, setName] = useState(account?.macos_guest_user || account?.username || "");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (!account?.macos_account_host || account.macos_credentials_ready) return children;
  async function submit(event) {
    event.preventDefault();
    if (busy) return;
    if (password !== confirmation) { setError("两次密码不一致"); return; }
    setBusy(true); setError("");
    try { await act("prepare-macos", { username:name, password, confirm_password:confirmation }); setPassword(""); setConfirmation(""); }
    catch (e) { setError(e.message); }
    finally { setBusy(false); }
  }
  return <main className="account-gate"><div className="account-brand"><img src={logo} alt="" />CCBox</div><section className="account-entry">
    <h1>设置 CCBox 与 macOS 共用账密</h1>
    <p className="account-entry-intro">{account.trusted_local ? "先设置 CCBox 用户名和密码，再继续客机安装。已有账户、文件和登录状态会保留。" : "输入当前 CCBox 密码，确认用于 macOS 客机。已有客机保留用户名和用户目录。"}</p>
    <form onSubmit={submit}><fieldset disabled={busy}>
      <label className="account-field">用户名<input required value={name} onChange={event => setName(event.target.value)} maxLength={31} pattern="[a-z][a-z0-9_]{0,30}" readOnly={Boolean(account.macos_guest_user)} autoComplete="username" /></label>
      <p className="field-help">小写字母开头，1–31 个小写字母、数字或下划线。密码使用 8–256 个英文键盘可输入的字符。</p>
      <PasswordField label={account.trusted_local ? "设置密码" : "当前 CCBox 密码"} value={password} onChange={setPassword} autoComplete={account.trusted_local ? "new-password" : "current-password"} />
      <PasswordField label="确认密码" value={confirmation} onChange={setConfirmation} autoComplete={account.trusted_local ? "new-password" : "current-password"} />
      {error && <p role="alert" className="account-error">{error}</p>}
      <button className="primary account-submit" disabled={busy}>{busy ? "正在保存…" : "保存账密并继续"}</button>
    </fieldset></form>
  </section></main>;
}

function PasswordField({
  label,
  value,
  onChange,
  autoComplete = "current-password",
}) {
  const [visible, setVisible] = useState(false);
  const id = useId();
  return (
    <div className="account-field">
      <label htmlFor={id}>{label}</label>
      <span className="password-input">
        <input
          id={id}
          type={visible ? "text" : "password"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          autoComplete={autoComplete}
          required
          maxLength={1024}
        />
        <button
          type="button"
          className="icon-button"
          aria-label={`${visible ? "隐藏" : "显示"}${label}`}
          onClick={() => setVisible(!visible)}
        >
          {visible ? <EyeOff size={16} /> : <Eye size={16} />}
        </button>
      </span>
    </div>
  );
}

export function AccountGate({ children }) {
  const { account, error: connectionError, refresh, act, deviceSupport, deviceAction, setDeviceNotice } = useAccount();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [rememberLogin, setRememberLogin] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [mode, setMode] = useState("register");
  const [resetCode, setResetCode] = useState("");
  const [notice, setNotice] = useState("");
  const automaticAttempt = useRef("");
  const gateAcknowledged = useRef(false);
  const trustedAttempt = useRef(false);
  const enterTrusted = async (action) => {
    setBusy(true); setError("");
    try { await trustedLogin(action); await refresh(); }
    catch (e) { setError(e.message); }
    finally { setBusy(false); }
  };
  useEffect(() => {
    if (account?.unlocked) { trustedAttempt.current = false; setRememberLogin(null); return; }
    if (!account || !localDesktop() || trustedAttempt.current || account.require_authentication) return;
    if (!(account.trusted_local || account.trusted_login)) return;
    trustedAttempt.current = true;
    void enterTrusted("resume");
  }, [account?.unlocked, account?.exists, account?.guest_system, account?.trusted_local, account?.trusted_login, account?.require_authentication]);
  useEffect(() => {
    if (!account || account.unlocked || connectionError || gateAcknowledged.current || !nativeAvailable()) return;
    // A working lock/setup screen is a valid upgraded UI. Waiting for the
    // user's password would incorrectly leave the native update unconfirmed.
    native("workbenchReady").then((accepted) => {
      if (accepted === true) gateAcknowledged.current = true;
    }).catch(() => {});
  }, [Boolean(account), account?.unlocked, connectionError]);
  useEffect(() => {
    const id = account?.devices?.automatic;
    if ((account?.trusted_login && localDesktop()) || !id || account.unlocked || account.require_authentication || !deviceSupport?.supported || deviceSupport.device_status?.automatic !== "available" || automaticAttempt.current === id) return;
    automaticAttempt.current = id;
    setBusy(true);
    deviceAction("unlock", "automatic").catch((e) => setError(e.message)).finally(() => setBusy(false));
  }, [account?.unlocked, account?.devices?.automatic, account?.require_authentication, deviceSupport]);
  if (account?.unlocked) return children;
  const creating = account && !account.exists;
  const online = Boolean(creating && account.online_enabled);
  const signingIn = online && mode === "login";
  const resetting = Boolean(account?.online_enabled && mode === "reset");
  const needsUsername = creating || resetting;
  const needsPasswordConfirmation = resetting || (creating && !signingIn);
  const switchMode = (next) => { setMode(next); setError(""); setNotice(""); setPassword(""); setConfirmation(""); setResetCode(""); };
  const openOnlineReset = () => {
    setUsername(account.online_username || username);
    switchMode("reset");
  };
  const rememberThisDevice = rememberLogin ?? Boolean(creating || account?.trusted_login || account?.devices?.automatic);
  const submit = async (event) => {
    event.preventDefault();
    if (busy) return;
    if (needsPasswordConfirmation && password !== confirmation) {
      setError("两次密码不一致");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      if (resetting) {
        await act("online-reset", { username, reset_code: resetCode, password, confirm_password: confirmation });
        switchMode("login");
        setNotice(creating ? "线上密码已重置，请用新密码登录。" : "线上密码已重置。此设备仍需使用原本机密码解锁。");
        return;
      }
      await act(
        creating ? signingIn ? "login" : "create" : "unlock",
        creating
          ? { username, password, confirm_password: confirmation }
          : { password },
      );
      if (localDesktop()) {
        try {
          await trustedLogin(rememberThisDevice ? "enable" : "disable", password);
          await refresh();
        } catch (e) {
          setDeviceNotice(`已登录，${rememberThisDevice ? "记住我未能开启" : "记住我未能关闭"}：${e.message}。可在「设置 → 账户」中重试。`);
        }
      } else if (creating && rememberThisDevice && deviceSupport?.supported && deviceSupport.device_status) {
        try { await deviceAction("enable", "automatic"); }
        catch (e) { setDeviceNotice(`账户已创建，记住登录未能开启：${e.message}。可在「设置 → 账户」中重试。`); }
      }
      setPassword("");
      setConfirmation("");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <main className="account-gate">
      {!isRemoteClient() && <AccountShowcase online={Boolean(account?.online_enabled)} />}
      <div className="account-brand">
        <img src={logo} alt="" />
        CCBox
      </div>
      <section
        className="account-entry"
        aria-label={resetting ? "重置线上账户密码" : creating ? isRemoteClient() ? "创建云端账户" : "创建本地账户" : "解锁 CCBox"}
      >
        {creating ? (
          <img className="account-welcome-logo" src={logo} alt="" />
        ) : (
          account && <AccountAvatar account={account} large />
        )}
        <h1>
          {resetting ? "重置线上账户密码" : creating
            ? signingIn ? "登录 CCBox 账户" : "注册 CCBox 账户"
            : account
              ? `欢迎回来，${account.username}`
              : "正在连接 CCBox"}
        </h1>
        <p className="account-entry-intro">
          {resetting
            ? creating ? "输入客服提供的重置码，为线上账户设置新密码。" : "输入客服提供的重置码。本机加密资料仍需使用原密码解锁。"
            : creating
            ? signingIn ? "使用已注册的 CCBox 账户登录这台电脑，密码同时用于解锁这台电脑上的数据。"
              : online ? "注册后可在其他电脑登录同一账户。密码同时用于解锁这台电脑上的数据。"
              : isRemoteClient() ? "在云端创建账户，保护这台主机上的工作与登录状态。" : "先注册用户名和密码，再选择客机系统并安装环境。macOS 客机会使用同一套账密。"
            : account
              ? account.trusted_local ? "本机工作区" : "输入密码，继续你的工作。"
              : isRemoteClient() ? "正在读取云端账户…" : "正在读取本机账户…"}
        </p>
        {account?.background_running && (
          <div className="account-background">
            <i />
            后台环境仍在运行，任务没有停止
          </div>
        )}
        {account?.trusted_local && <div className="account-trust-choice">
          <p>{localDesktop() ? "使用当前系统用户的本机信任凭据进入工作区。" : "请回到受信任的桌面设置账户密码，再从此处登录。"}</p>
          {localDesktop() && <button className="quiet-button" disabled={busy} onClick={() => enterTrusted("resume")}>{busy ? "正在进入…" : "重新进入本机工作区"}</button>}
        </div>}
        {account?.trusted_local && error && <p className="account-error" role="alert">{error}</p>}
        {account && !account.trusted_local && (
          <form onSubmit={submit}>
            <fieldset disabled={busy}>
              {needsUsername && (
                <><label className="account-field">
                  用户名
                  <input
                    autoFocus
                    autoComplete="username"
                    maxLength={account.macos_account_host && !signingIn && !resetting ? 31 : online ? 32 : 40}
                    pattern={account.macos_account_host && !signingIn && !resetting ? "[a-z][a-z0-9_]{0,30}" : undefined}
                    required
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder={account.macos_account_host ? "例如：shihao" : "你希望怎样被称呼"}
                  />
                </label>
                {account.macos_account_host && !signingIn && !resetting && <p className="field-help">与 macOS 客机共用：小写字母开头，1–31 个小写字母、数字或下划线。</p>}
                {online && !account.macos_account_host && mode === "register" && <p className="field-help">3–32 个字母、数字、下划线、连字符或点。</p>}</>
              )}
              {resetting && (
                <label className="account-field">
                  重置码
                  <input autoComplete="one-time-code" required maxLength={20} value={resetCode} onChange={(e) => setResetCode(e.target.value)} placeholder="XXXX-XXXX-XXXX" />
                </label>
              )}
              <PasswordField
                label={signingIn ? "账户密码" : resetting ? "新密码" : creating ? "设置密码" : "账户密码"}
                value={password}
                onChange={setPassword}
                autoComplete={needsPasswordConfirmation ? "new-password" : "current-password"}
              />
              {needsPasswordConfirmation && (
                <PasswordField
                  label="确认密码"
                  value={confirmation}
                  onChange={setConfirmation}
                  autoComplete="new-password"
                />
              )}
              {creating && account.macos_account_host && !signingIn && !resetting && <p className="field-help">密码使用 8–256 个英文键盘可输入的字母、数字、空格或符号。安装中断后继续使用这套账密。</p>}
              {(error || connectionError) && (
                <p className="account-error" role="alert">
                  {error || connectionError}
                </p>
              )}
              {notice && <p className="field-help" role="status">{notice}</p>}
              {!creating && Object.values(deviceSupport?.device_status || {}).includes("missing") && (
                <p className="field-help" role="status">当前目录的快捷登录凭据缺失或已失效。请用密码解锁，再到「设置 → 账户 → 登录与解锁」重新开启。</p>
              )}
              {!creating && Object.values(deviceSupport?.device_status || {}).includes("unavailable") && (
                <p className="field-help" role="status">暂时无法检查系统钥匙串，请先用密码解锁。快捷登录授权记录已保留。</p>
              )}
              {deviceSupport?.supported && !deviceSupport.device_status && (
                <p className="field-help" role="status">当前应用启动器需要更新，暂时无法检查快捷登录状态。请先用密码进入，并重新安装完整的 CCBox 应用。</p>
              )}
              {!resetting && (localDesktop() || (creating && deviceSupport?.supported && deviceSupport.device_status)) && (
                <label className="account-device-option check-line">
                  <input type="checkbox" checked={rememberThisDevice} onChange={(e) => setRememberLogin(e.target.checked)} />
                  <span><strong>{localDesktop() ? "记住我" : "记住登录"}</strong><small>{localDesktop() ? "信任此电脑，下次用当前系统用户打开时直接进入。" : "下次打开直接进入。"}</small></span>
                </label>
              )}
              {!creating && deviceSupport?.supported && credentialPresent(deviceSupport.device_status?.biometric) && account.devices?.biometric && (
                <button type="button" className="quiet-button account-submit" disabled={busy}
                  onClick={async () => {
                    setBusy(true); setError("");
                    try { await deviceAction("unlock", "biometric"); }
                    catch (e) { setError(e.message); }
                    finally { setBusy(false); }
                  }}>
                  <Fingerprint size={18} />使用 Touch ID 解锁
                </button>
              )}
              <button
                className="primary account-submit"
                disabled={busy || !password || (resetting && !resetCode.trim()) || (needsUsername && !username.trim()) ||
                  (creating && !resetting && !localDesktop() && nativeAvailable() && deviceSupport === null)}
              >
                {busy ? creating ? "正在处理…" : "正在解锁…"
                  : signingIn ? "登录" : resetting ? "重置密码" : online ? "注册" : creating ? isRemoteClient() ? "创建云端账户" : "创建本地账户" : "解锁 CCBox"}
              </button>
              {online && <div className="account-mode-links">
                {mode === "register" && <button type="button" className="text-button" onClick={() => switchMode("login")}>已有 CCBox 账户？登录</button>}
                {mode === "login" && <><button type="button" className="text-button" onClick={() => switchMode("register")}>没有账户？注册</button>
                  <button type="button" className="text-button" onClick={openOnlineReset}>忘记密码？</button></>}
              </div>}
              {resetting && <button type="button" className="text-button" onClick={() => switchMode("login")}>返回登录</button>}
              {!creating && account.online_enabled && !resetting && <button type="button" className="text-button" onClick={openOnlineReset}>忘记密码？</button>}
            </fieldset>
          </form>
        )}
        {!account && connectionError && (
          <>
            <p role="alert" className="account-error">
              {connectionError}
            </p>
            <button onClick={() => refresh().catch(() => {})}>重新连接</button>
          </>
        )}
        <p className="account-local-note">
          <LockKeyhole size={14} />
          {isRemoteClient() ? "账户资料保存在当前云端主机" : online || account?.online_username ? "本机数据只保存在这台设备上，CCBox 服务器只保存账户与授权" : "账户资料仅保存在这台设备上"}
        </p>
        {nativeAvailable() && (
          <button
            type="button"
            className="text-button account-exit-app"
            onClick={() => native("quitNow").catch((e) => setError(e.message))}
          >
            {account?.background_running ? "退出应用，后台继续运行" : "退出 CCBox"}
          </button>
        )}
      </section>
    </main>
  );
}

export function AccountIdentity({ onStatus }) {
  const { account, deviceNotice, setDeviceNotice } = useAccount();
  useEffect(() => {
    if (deviceNotice) { onStatus(deviceNotice); setDeviceNotice(""); }
  }, [deviceNotice]);
  return (
    <div className="sidebar-account-identity">
      <AccountAvatar account={account} />
      <span className="sidebar-account-name">
        <strong title={account.username}>{account.username}</strong>
        <small>{account.trusted_local ? "受信任的本机" : isRemoteClient() ? "云端账户" : "本地账户"}</small>
      </span>
    </div>
  );
}

export function AccountSettings({ busy: environmentBusy, onDirty, onConfirm }) {
  const { account, act, refresh } = useAccount();
  const [name, setName] = useState(account.username),
    [avatar, setAvatar] = useState(account.avatar || "");
  const [oldPassword, setOldPassword] = useState(""),
    [password, setPassword] = useState(""),
    [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const [feedbackTarget, setFeedbackTarget] = useState("profile");
  const file = useRef();
  const changed =
    name !== account.username || avatar !== (account.avatar || "");
  useEffect(() => {
    onDirty?.(changed || Boolean(oldPassword || password || confirmation));
  }, [changed, oldPassword, password, confirmation]);
  useEffect(() => () => onDirty?.(false), []);
  const upload = async (event) => {
    setFeedbackTarget("profile");
    const selected = event.target.files?.[0];
    event.target.value = "";
    if (!selected) return;
    if (
      selected.size > 2 * 1024 * 1024 ||
      !["image/png", "image/jpeg"].includes(selected.type)
    ) {
      setError("请选择不超过 2 MB 的 PNG 或 JPEG 图片");
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      setAvatar(reader.result);
      setError("");
      setMessage("");
    };
    reader.readAsDataURL(selected);
  };
  const save = async (event, changingPassword = false) => {
    event.preventDefault();
    if (busy) return;
    setFeedbackTarget(changingPassword ? "password" : "profile");
    if (changingPassword && password !== confirmation) {
      setError("两次密码不一致");
      return;
    }
    setBusy(true);
    setError("");
    setMessage("");
    try {
      if (changingPassword && account.trusted_local) {
        await trustedLogin("protect", password);
        await refresh();
        setPassword(""); setConfirmation("");
        setMessage("已设置密码，下次打开 CCBox 时使用密码登录。");
        return;
      }
      const value = await act(
        changingPassword ? "password" : "profile",
        changingPassword
          ? {
              old_password: oldPassword,
              password,
              confirm_password: confirmation,
            }
          : { username: name, avatar },
      );
      if (changingPassword) {
        setOldPassword("");
        setPassword("");
        setConfirmation("");
        setMessage(value.macos_password_pending ? "CCBox 密码已修改。客机密码待同步，下次启动时自动重试；同步完成前客机可能仍需原密码。" : "密码已修改，应用登录状态保持不变");
      } else {
        setName(value.username);
        setAvatar(value.avatar || "");
        setMessage("账户资料已保存");
      }
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  const feedback = (
    <>
      {error && (
        <p className="account-error" role="alert">
          {error}
        </p>
      )}
      {message && (
        <p className="account-saved" role="status">
          <Check size={15} />
          {message}
        </p>
      )}
    </>
  );
  if (account.macos_account_host && account.guest_system === "macos" && !account.macos_credentials_ready) return <MacOSAccountGate />;
  return (
    <div className="account-settings">
      <div className="section-title">
        <h3>账户</h3>
        <p>给工作空间一点你的样子。</p>
      </div>
      <form onSubmit={(event) => save(event)}>
        <fieldset disabled={busy || environmentBusy}>
          <div className="account-profile-heading">
            <AccountAvatar account={{ username: name, avatar }} large />
            <div>
              <button
                type="button"
                className="quiet-button"
                onClick={() => file.current.click()}
              >
                <Camera size={16} />
                更换头像
              </button>
              {avatar && (
                <button
                  type="button"
                  className="text-button"
                  onClick={() => setAvatar("")}
                >
                  恢复默认
                </button>
              )}
              <p className="field-help">
                PNG 或 JPEG，最多 2 MB，自动居中裁切。
              </p>
            </div>
          </div>
          <input
            ref={file}
            type="file"
            accept="image/png,image/jpeg"
            hidden
            onChange={upload}
            aria-label="选择头像图片"
          />
          <label className="account-field">
            用户名
            <input
              value={name}
              maxLength={40}
              autoComplete="username"
              onChange={(e) => setName(e.target.value)}
              required
              readOnly={account.macos_username_bound}
            />
          </label>
          {account.macos_username_bound && <p className="field-help">用户名已用于 macOS 客机，保留该名称以继续使用原账户和文件。</p>}
          <button
            className="primary"
            disabled={!changed || !name.trim() || busy}
          >
            <Check size={16} />
            保存资料
          </button>
        </fieldset>
      </form>
      {feedbackTarget === "profile" && feedback}
      <div className="account-section-divider" />
      <form onSubmit={(event) => save(event, true)}>
        <fieldset disabled={busy || environmentBusy}>
          <h4>账户密码</h4>
          <p className="field-help">
            修改密码不会清除 Claude 的登录状态。{account.macos_credentials_ready && "CCBox 与 macOS 客机共用密码，客机关闭时会在下次启动后同步。"}
          </p>
          {!account.trusted_local && <PasswordField
            label="当前密码"
            value={oldPassword}
            onChange={setOldPassword}
          />}
          <PasswordField
            label="新密码"
            value={password}
            onChange={setPassword}
            autoComplete="new-password"
          />
          <PasswordField
            label="确认新密码"
            value={confirmation}
            onChange={setConfirmation}
            autoComplete="new-password"
          />
          <button
            className="quiet-button"
            disabled={busy || (!account.trusted_local && !oldPassword) || !password || !confirmation}
          >
            {account.trusted_local ? "设置密码并关闭免账号登录" : "修改密码"}
          </button>
        </fieldset>
      </form>

      {feedbackTarget === "password" && feedback}
      {localDesktop() && !account.trusted_local && <TrustedDeviceSettings />}
      {!account.trusted_local && <AccountDeviceSettings />}
      {account.online_enabled && !account.trusted_local && <OnlineAccountSettings />}
      {!account.trusted_local && <>
        <div className="account-section-divider" />
        <button type="button" className="quiet-button" disabled={busy || environmentBusy} onClick={async () => {
          if (!(await onConfirm({
            title: isRemoteClient() ? "退出云端账户？" : "退出本地账户？",
            body: "这会停止 CCBox 环境和后台任务，并撤销本机快捷登录。请先保存文件。",
            confirm: "停止环境并退出账户",
          }))) return;
          setBusy(true); setError(""); setMessage(""); setFeedbackTarget("logout");
          try { await act("logout"); }
          catch (error) { setError(error.message); }
          finally { setBusy(false); }
        }}><LogOut size={16} />退出账户</button>
        {feedbackTarget === "logout" && feedback}
      </>}
      <p className="account-settings-note">
        <LockKeyhole size={15} />
        {isRemoteClient() ? "账户、应用资料与登录凭据保存在当前云端主机。" : account.online_enabled ? "本机资料与登录凭据只保存在这台设备上；CCBox 服务器只保存线上账户与授权。" : "一个本地账户，一个持久工作环境。资料与凭据不上传到 CCBox 服务器。"}
      </p>
    </div>
  );
}

function TrustedDeviceSettings() {
  const { account, refresh } = useAccount();
  const enabled = Boolean(account.trusted_login || account.devices?.automatic);
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const update = async (event) => {
    event.preventDefault();
    setBusy(true); setMessage("");
    try {
      await trustedLogin(enabled ? "disable" : "enable", password);
      setPassword("");
      await refresh();
    } catch (error) { setMessage(error.message); }
    finally { setBusy(false); }
  };
  return <form onSubmit={update}>
    <fieldset disabled={busy}>
      <h4>记住我</h4>
      <p className="field-help">{enabled ? "已信任此电脑，下次用当前系统用户打开时自动进入。退出账户会撤销信任。" : "验证一次账户密码，信任此电脑，下次用当前系统用户打开时自动进入。"}</p>
      {!enabled && <PasswordField label="验证账户密码" value={password} onChange={setPassword} autoComplete="current-password" />}
      <button className="quiet-button" disabled={!enabled && !password}>{enabled ? "关闭记住我" : "信任此电脑"}</button>
      {message && <p className="account-error" role="alert">{message}</p>}
    </fieldset>
  </form>;
}

function AccountDeviceSettings() {
  const { account, deviceSupport, deviceAction, recheckDevices } = useAccount();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  if (!deviceSupport?.supported) return null;
  if (!deviceSupport.device_status) return <section className="account-device-settings" aria-label="登录与解锁">
    <h4>登录与解锁</h4>
    <p className="field-help" role="status">当前应用启动器需要更新，无法检查快捷登录状态。请重新安装完整的 CCBox 应用后再设置；现有授权已保留。</p>
  </section>;
  const states = deviceSupport.device_status || {};
  const description = (kind, fallback) => states[kind] === "missing"
    ? "当前目录的凭据缺失或已失效，请重新开启。"
    : states[kind] === "unavailable"
      ? "暂时无法检查系统钥匙串，授权记录已保留。请稍后重新检查。"
      : states[kind] === "authentication_required"
        ? "凭据需要通过 Touch ID 验证，解锁时会请求指纹。"
        : fallback;
  const change = async (kind, enabled) => {
    setBusy(true); setError(""); setMessage("");
    try {
      await deviceAction(enabled ? "enable" : "disable", kind);
      setMessage(enabled ? "已开启，解锁凭据保存在这台 Mac 的系统钥匙串中" : "已关闭快捷登录授权");
    } catch (e) { setError(e.message); }
    finally { setBusy(false); }
  };
  return <section className="account-device-settings" aria-label="登录与解锁">
    <h4>登录与解锁</h4>
    {!localDesktop() && <label className="account-device-option check-line">
      <input type="checkbox" checked={Boolean(account.devices?.automatic) && states.automatic === "available"} disabled={busy || states.automatic === "unavailable"}
        onChange={(event) => change("automatic", event.target.checked)} />
      <span><strong>记住登录</strong><small>{description("automatic", "重新打开时可自动进入。")}</small></span>
    </label>}
    <label className="account-device-option check-line">
      <input type="checkbox" checked={Boolean(account.devices?.biometric) && credentialPresent(states.biometric)} disabled={busy || states.biometric === "unavailable" || (!deviceSupport.biometric_available && !credentialPresent(states.biometric))}
        onChange={(event) => change("biometric", event.target.checked)} />
      <span><strong>使用 Touch ID 解锁</strong><small>{description("biometric", deviceSupport.biometric_available ? "需要验证账户时，可用系统指纹进入。" : "请先在 macOS 系统设置中设置 Touch ID。")}</small></span>
    </label>
    {Object.values(states).includes("unavailable") && <button type="button" className="quiet-button" disabled={busy} onClick={recheckDevices}>重新检查快捷登录</button>}
    <p className="field-help">退出账户会撤销本机快捷登录授权。密码始终可以用于解锁。</p>
    {error && <p className="account-error" role="alert">{error}</p>}
    {message && <p className="account-saved" role="status">{message}</p>}
  </section>;
}

// Brand panel beside the sign-in form on wide windows; hidden when narrow.
function AccountShowcase({ online }) {
  return <aside className="account-showcase" aria-label="CCBox 简介">
    <div className="account-showcase-brand"><img src={logo} alt="" />CCBox</div>
    <div className="account-showcase-body">
      <img className="account-showcase-logo" src={logo} alt="" />
      <h2>为 Claude 准备的<br /><span>独立工作区</span></h2>
      <ul>
        <li><ShieldCheck size={17} aria-hidden="true" /><span><strong>隔离环境</strong>浏览器、终端和编辑器运行在独立的客户机系统里。</span></li>
        <li><Globe2 size={17} aria-hidden="true" /><span><strong>内置代理</strong>导入自己的订阅或节点即可联网，无需额外的代理软件。</span></li>
        {online
          ? <li><LockKeyhole size={17} aria-hidden="true" /><span><strong>一个账户，多台电脑</strong>授权跟随账户；本机数据与凭据只保存在这台设备上。</span></li>
          : <li><LockKeyhole size={17} aria-hidden="true" /><span><strong>本机账户</strong>账户资料与凭据只保存在这台设备上。</span></li>}
      </ul>
    </div>
  </aside>;
}

const onlineDate = (seconds) => new Date(seconds * 1000).toLocaleDateString("zh-CN", { year: "numeric", month: "long", day: "numeric" });

// Settings › 账户: the online CCBox account this device is signed in to.
function OnlineAccountSettings() {
  const { act } = useAccount();
  const [view, setView] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [mode, setMode] = useState("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const load = async (body) => {
    setView(await api("/account/online", body));
  };
  useEffect(() => { load().catch((e) => setError(e.message)); }, []);
  const run = async (task, done) => {
    setBusy(true); setError(""); setMessage("");
    try { await task(); if (done) setMessage(done); }
    catch (e) { setError(e.message); }
    finally { setBusy(false); }
  };
  const link = (event) => {
    event.preventDefault();
    if (mode === "register" && password !== confirmation) { setError("两次密码不一致"); return; }
    void run(async () => {
      await act("link", { kind: mode, username, password, confirm_password: confirmation });
      setPassword(""); setConfirmation("");
      await load();
    }, mode === "register" ? "已注册并登录线上账户" : "已登录线上账户");
  };
  if (!view) return <section className="account-online-settings" aria-label="线上账户"><h4>CCBox 线上账户</h4>
    {error ? <p className="account-error" role="alert">{error}</p> : <p className="field-help" role="status">正在读取…</p>}</section>;
  const info = view.account;
  return <section className="account-online-settings" aria-label="线上账户">
    <h4>CCBox 线上账户</h4>
    {view.linked ? <>
      <p className="field-help">
        已登录 <strong>{view.username}</strong>。
        {info ? info.active ? `授权有效至 ${onlineDate(info.expires)}。` : "账户暂无有效授权，请在「设置 → 授权」兑换激活码。" : view.error || ""}
      </p>
      {info?.devices?.length > 0 && <ul className="account-online-devices">
        {info.devices.map((device) => <li key={device.id}>
          <span><strong>{device.name || "未命名设备"}{device.current ? "（当前设备）" : ""}</strong>
            <small>{[device.platform, device.seen ? `最近使用 ${onlineDate(device.seen)}` : ""].filter(Boolean).join(" · ")}</small></span>
          {!device.current && <button type="button" className="quiet-button" disabled={busy}
            onClick={() => run(() => load({ action: "remove-device", target: device.id }), "已移除该设备")}>移除</button>}
        </li>)}
      </ul>}
      {info && <p className="field-help">每个账户最多同时登录 {info.max_devices} 台设备。</p>}
      <div className="account-online-actions">
        <button type="button" className="quiet-button" disabled={busy} onClick={() => run(() => load({ action: "refresh" }), "已同步授权")}>同步授权</button>
        <button type="button" className="quiet-button" disabled={busy} onClick={() => run(async () => { await act("online-logout"); await load(); }, "已退出线上账户，本机授权在到期前仍可使用")}>退出线上账户</button>
      </div>
    </> : <form onSubmit={link}>
      <fieldset disabled={busy}>
        <p className="field-help">{view.error ? `${view.error}。` : ""}登录或注册 CCBox 账户后，授权跟随账户，可在其他电脑登录使用。本机解锁密码保持不变。</p>
        <label className="account-field">
          用户名
          <input autoComplete="username" maxLength={32} required value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
        <PasswordField label={mode === "register" ? "设置账户密码" : "账户密码"} value={password} onChange={setPassword} autoComplete={mode === "register" ? "new-password" : "current-password"} />
        {mode === "register" && <PasswordField label="确认账户密码" value={confirmation} onChange={setConfirmation} autoComplete="new-password" />}
        <div className="account-online-actions">
          <button className="quiet-button" disabled={!username.trim() || !password}>{mode === "register" ? "注册并登录" : "登录"}</button>
          <button type="button" className="text-button" onClick={() => { setMode(mode === "register" ? "login" : "register"); setError(""); setConfirmation(""); }}>
            {mode === "register" ? "已有账户？登录" : "没有账户？注册"}
          </button>
        </div>
      </fieldset>
    </form>}
    {error && <p className="account-error" role="alert">{error}</p>}
    {message && <p className="account-saved" role="status">{message}</p>}
  </section>;
}
