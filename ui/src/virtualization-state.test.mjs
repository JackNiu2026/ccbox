import assert from "node:assert/strict";
import test from "node:test";
import { actionConfirmation, hostAllowsInstall, isLinuxGuest, virtualizationActions } from "./virtualization-state.mjs";

test("Linux guests are recognized; native guests are not", () => {
  for (const system of ["debian-13", "ubuntu-22.04"]) assert.ok(isLinuxGuest(system));
  for (const system of ["macos", "windows-11", undefined, ""]) assert.ok(!isLinuxGuest(system));
});

test("a ready or missing report offers no actions", () => {
  assert.deepEqual(virtualizationActions(null), []);
  assert.deepEqual(virtualizationActions({ ready: true, can_enable: true }), []);
});

test("actions follow the report, enable first", () => {
  const all = virtualizationActions({ ready: false, can_enable: true, needs_restart: true, firmware_restart: true });
  assert.deepEqual(all.map((a) => a.action), ["enable", "restart", "firmware"]);
  assert.equal(all.filter((a) => a.primary).length, 1);
  assert.equal(all[0].primary, true);
  const restartOnly = virtualizationActions({ ready: false, needs_restart: true });
  assert.deepEqual(restartOnly.map((a) => [a.action, a.primary]), [["restart", true]]);
  assert.deepEqual(virtualizationActions({ ready: false, state: "kvm-permission" }), []);
});

test("every restart asks for confirmation", () => {
  for (const action of ["restart", "firmware"]) assert.match(actionConfirmation[action], /保存其他程序中的工作/);
});

test("install is blocked only by a definite not-ready Linux check", () => {
  assert.ok(hostAllowsInstall(null));
  assert.ok(hostAllowsInstall({ guest_system: "ubuntu-22.04" }));
  assert.ok(hostAllowsInstall({ guest_system: "ubuntu-22.04", host_ready: true }));
  assert.ok(!hostAllowsInstall({ guest_system: "debian-13", host_ready: false }));
  assert.ok(hostAllowsInstall({ guest_system: "windows-11", host_ready: false }));
});
