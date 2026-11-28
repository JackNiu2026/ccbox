import assert from "node:assert/strict";
import test from "node:test";
import { terminalAccountSelection } from "./terminal-menu-state.mjs";

const ready = (id, enabled, handoff_group = "") => ({ id, enabled, handoff_group, status: "ready" });

test("an empty account list keeps the environment login and its existing policy", () => {
  const state = terminalAccountSelection([], "", "allow");
  assert.equal(state.account, "auto");
  assert.equal(state.hasBackup, false);
  assert.equal(state.autoHandoff, undefined);
});

test("only enabled, ready backups in the same group can continue", () => {
  const accounts = [
    ready("a", true, "team"), ready("b", true, "other"),
    ready("manual", false, "team"),
    { ...ready("expired", true, "team"), status: "signed_out" },
  ];
  assert.equal(terminalAccountSelection(accounts, "a", "allow").hasBackup, false);
  assert.equal(terminalAccountSelection(accounts, "manual", "allow").hasBackup, true);
  const all = [...accounts, ready("backup", true, "team")];
  assert.equal(terminalAccountSelection(all, "auto", "allow").autoHandoff, true);
  assert.equal(terminalAccountSelection(all, "auto", "allow", false).autoHandoff, false);
  assert.equal(terminalAccountSelection(all, "a", "allow").autoHandoff, false);
  assert.equal(terminalAccountSelection(all, "a", "ask", true).autoHandoff, true);
});
