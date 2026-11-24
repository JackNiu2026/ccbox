import assert from "node:assert/strict";
import { test } from "node:test";
import { BrowserVisibility } from "./browser-visibility.mjs";
import * as browserTargets from "./browser-visibility.mjs";

test("closing a navigated popup returns focus to its original live tab", () => {
  const previous = { targetId: "popup", openerId: "open", browserContextId: "private" };
  const popup = browserTargets.rememberOpener({ targetId: "popup", browserContextId: "private" }, previous);
  const tabs = new Map([
    ["fallback", { targetId: "fallback" }],
    ["open", { targetId: "open", browserContextId: "private" }],
  ]);
  assert.equal(browserTargets.targetAfterClose(tabs, popup), "open");
  assert.equal(browserTargets.targetAfterClose(tabs, { ...popup, browserContextId: "other" }), "fallback");
  assert.equal(browserTargets.rememberOpener({ targetId: "new" }, previous).openerId, undefined);
});

function fixture() {
  const windows = new Map([[1, { windowState: "fullscreen", left: 0, top: 0, width: 1200, height: 800 }]]);
  const targets = new Map([["one", 1], ["two", 1]]);
  const calls = [];
  let legacyViewer = false;
  const cdp = async (method, input) => {
    calls.push({ method, ...input });
    if (method === "Browser.getWindowForTarget") {
      const windowId = targets.get(input.targetId);
      return { windowId, bounds: { ...windows.get(windowId) } };
    }
    if (method === "Browser.getWindowBounds") return { bounds: { ...windows.get(input.windowId) } };
    const current = windows.get(input.windowId);
    assert.ok(current);
    if (input.bounds.windowState === "minimized") assert.equal(current.windowState, "normal");
    Object.assign(current, input.bounds);
  };
  const visibility = new BrowserVisibility(cdp, () => [...targets.keys()], async () => !legacyViewer);
  return { visibility, windows, targets, calls, setLegacyViewer: value => { legacyViewer = value; } };
}

test("browser visibility restores fullscreen and processes shared windows once", async () => {
  const f = fixture();
  await f.visibility.set(false);
  assert.equal(f.windows.get(1).windowState, "minimized");
  assert.equal(f.calls.filter(call => call.method === "Browser.setWindowBounds").length, 2);
  await f.visibility.set(false);
  assert.equal(f.calls.filter(call => call.method === "Browser.setWindowBounds").length, 2);
  await f.visibility.set(true);
  assert.equal(f.windows.get(1).windowState, "fullscreen");
});

test("browser visibility restores a normal popup fullscreen without affecting closed windows", async () => {
  const f = fixture();
  const bounds = { windowState: "normal", left: 40, top: 80, width: 900, height: 600 };
  f.windows.set(2, { ...bounds }); f.targets.set("popup", 2);
  await f.visibility.set(false);
  f.targets.delete("one"); f.targets.delete("two"); f.windows.delete(1);
  await f.visibility.set(true);
  assert.equal(f.windows.get(2).windowState, "fullscreen");
});

test("a legacy viewer keeps a newly visible Chromium window fullscreen", async () => {
  const f = fixture();
  f.windows.set(2, { windowState: "normal" }); f.targets.set("popup", 2);
  f.setLegacyViewer(true);
  await f.visibility.set(false);
  assert.equal(f.windows.get(2).windowState, "fullscreen");
  assert.equal(f.calls.some(call => call.bounds?.windowState === "minimized"), false);
});

test("a newer browser viewer restores a hide that is already running", async () => {
  let release, entered;
  const blocked = new Promise(resolve => { entered = resolve; });
  const ready = new Promise(resolve => { release = resolve; });
  let state = "fullscreen";
  const visibility = new BrowserVisibility(async (method, input) => {
    if (method === "Target.activateTarget") return {};
    if (method === "Browser.getWindowForTarget") return { windowId: 1, bounds: { windowState: state } };
    if (method === "Browser.getWindowBounds") return { bounds: { windowState: state } };
    if (input.bounds.windowState === "minimized") { entered(); await ready; }
    state = input.bounds.windowState;
  }, () => ["one"]);
  const hidden = visibility.set(false);
  await blocked;
  const activated = visibility.activate("one");
  const shown = visibility.set(true);
  release();
  await Promise.all([hidden, activated, shown]);
  assert.equal(state, "fullscreen");
});
