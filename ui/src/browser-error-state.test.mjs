import assert from "node:assert/strict";
import test from "node:test";
import * as workspace from "./workspace-state.mjs";

test("browser connection errors are shown only in the visible running browser view", () => {
  const visible = { browserError: "disconnected", browserVisible: true, running: true };
  assert.equal(workspace.shouldShowBrowserError(visible), true);
  for (const hidden of [
    { browserVisible: false },
    { managementPage: "accounts" },
    { running: false },
    { connectionError: "workbench disconnected" },
    { browserError: "" },
  ]) assert.equal(workspace.shouldShowBrowserError({ ...visible, ...hidden }), false);
});
