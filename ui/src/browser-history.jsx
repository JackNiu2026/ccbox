import React, { useEffect, useId, useState } from "react";
import { ArrowUpRight, Globe2, History, ScanEye } from "lucide-react";
import { api } from "./api";
import { translate as tr } from "./message-catalog.mjs";
import { Modal } from "./modal";

function historyRequest(tabID, method = "GET", signal) {
  return api("/browser/history?id=" + encodeURIComponent(tabID), undefined, method, {}, { signal });
}

function matchingAddresses(addresses, input) {
  const query = input.trim().replace(/^https?:\/\//i, "").toLowerCase();
  return query ? addresses.filter(address => address.replace(/^https?:\/\//, "").toLowerCase().includes(query)) : addresses;
}

export function BrowserAddressBar({ tab, ready, historyEnabled, onNavigate }) {
  const url = tab.url === "about:blank" ? "" : tab.url || "";
  const [address, setAddress] = useState(url);
  const [focused, setFocused] = useState(false);
  const [edited, setEdited] = useState(false);
  const [addresses, setAddresses] = useState([]);
  const [active, setActive] = useState(-1);
  const listID = useId();
  useEffect(() => {
    setAddress(url);
    setEdited(false);
    setActive(-1);
  }, [tab.id, url]);
  useEffect(() => {
    setAddresses([]);
    if (!historyEnabled || !focused) return;
    const request = new AbortController();
    historyRequest(tab.id, "GET", request.signal).then(result => {
      if (!request.signal.aborted) setAddresses(result.addresses);
    }).catch(() => {});
    return () => request.abort();
  }, [tab.id, focused, historyEnabled]);
  const matches = historyEnabled && focused && edited && address.trim()
    ? matchingAddresses(addresses, address).slice(0, 6) : [];
  const expanded = matches.length > 0;
  const navigate = value => {
    setEdited(false);
    setActive(-1);
    setAddress(value);
    onNavigate(value);
  };
  return <form className="address-bar" onSubmit={event => {
    event.preventDefault();
    navigate(matches[active] || address);
  }}>
    {tab.incognito ? <span className="browser-private-badge" title="关闭全部无痕标签后清理此会话"><ScanEye size={13} />无痕</span> : <Globe2 size={13} />}
    <input aria-label="网页地址或搜索" value={address}
      role={historyEnabled ? "combobox" : undefined}
      aria-autocomplete={historyEnabled ? "list" : undefined}
      aria-expanded={historyEnabled ? expanded : undefined}
      aria-controls={expanded ? listID : undefined}
      aria-activedescendant={expanded && matches[active] ? listID + "-" + active : undefined}
      onChange={event => { setAddress(event.target.value); setEdited(true); setActive(-1); }}
      onFocus={event => { event.target.select(); setFocused(true); }}
      onBlur={() => { setFocused(false); setEdited(false); setActive(-1); }}
      onKeyDown={event => {
        if (event.nativeEvent.isComposing || !expanded) return;
        if (event.key === "ArrowDown" || event.key === "ArrowUp") {
          event.preventDefault();
          setActive(index => index < 0 ? (event.key === "ArrowUp" ? matches.length - 1 : 0)
            : (index + (event.key === "ArrowUp" ? -1 : 1) + matches.length) % matches.length);
        } else if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          setEdited(false);
          setActive(-1);
        }
      }}
      placeholder={ready ? "输入网址或搜索，在 CCBox 内打开" : "启动环境后浏览网页"}
      disabled={!ready} spellCheck="false" autoComplete="off" />
    <button className="address-go" type="submit" aria-label="打开地址" disabled={!ready}>
      <ArrowUpRight size={15} />
    </button>
    {expanded && <ul className="browser-address-suggestions" id={listID} role="listbox" aria-label={tr("地址联想")}>
      {matches.map((item, index) => <li key={item} id={listID + "-" + index} role="option" aria-selected={active === index}
        onPointerDown={event => event.preventDefault()} onClick={() => navigate(item)}>
        <History size={14} /><span>{item}</span>
      </li>)}
    </ul>}
  </form>;
}

export function BrowserHistoryDialog({ tabID, onClose, onNavigate }) {
  const [addresses, setAddresses] = useState(null);
  const [query, setQuery] = useState("");
  const [failed, setFailed] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  useEffect(() => {
    const request = new AbortController();
    historyRequest(tabID, "GET", request.signal).then(result => setAddresses(result.addresses)).catch(() => {
      if (!request.signal.aborted) setFailed(true);
    });
    return () => request.abort();
  }, [tabID]);
  const clear = async () => {
    setClearing(true);
    setFailed(false);
    try {
      await historyRequest(tabID, "DELETE");
      setAddresses([]);
    } catch { setFailed(true); }
    finally { setClearing(false); setConfirmClear(false); }
  };
  const matches = matchingAddresses(addresses || [], query);
  return <Modal title={tr("浏览历史")} onClose={onClose}
    footer={addresses?.length > 0 && (confirmClear ? <>
      <button type="button" disabled={clearing} onClick={() => setConfirmClear(false)}>{tr("取消")}</button>
      <button type="button" disabled={clearing} onClick={clear}>{tr("确认清除浏览历史")}</button>
    </> : <button type="button" onClick={() => setConfirmClear(true)}>{tr("清除浏览历史")}</button>)}>
    <label className="browser-history-search">
      {tr("搜索浏览历史")}
      <input type="search" value={query} onChange={event => setQuery(event.target.value)} />
    </label>
    {failed && <p role="alert">{tr("浏览历史暂不可用，请重新打开再试。")}</p>}
    {!failed && addresses === null && <p role="status">{tr("正在读取浏览历史…")}</p>}
    {!failed && addresses !== null && !matches.length && <p role="status">{tr(query ? "未找到匹配的记录" : "暂无浏览历史")}</p>}
    {matches.length > 0 && <ul className="browser-history-list">
      {matches.map(address => <li key={address}><button type="button" title={address}
        onClick={() => { onClose(); onNavigate(address); }}>{address}</button></li>)}
    </ul>}
  </Modal>;
}
