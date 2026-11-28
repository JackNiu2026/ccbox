import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { BrowserHistory, historyAddress } from "./browser-history.mjs";

const regular = { type: "page" };
const frame = url => ({ url });

test("only replayable public addresses can be recorded", () => {
  assert.equal(historyAddress("https://example.com/docs?utm_source=a"), "https://example.com/docs");
  for (const address of [
    "chrome://history", "http://127.0.0.1:3000/", "http://editor.local/",
    "https://user:secret@example.com/", "https://accounts.example.com/",
    "https://example.com/oauth/callback", "https://example.com/%6cogin",
    "https://example.com/?code=secret", "https://example.com/#access_token=secret",
    "https://example.com/reset-password/SECRET", "https://example.com/a%2flogin",
    "https://example.com/search?q=private",
  ]) assert.equal(historyAddress(address), "", address);
});

function temporaryHistory(t) {
  const dir = mkdtempSync(join(tmpdir(), "ccbox-history-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return join(dir, "history.json");
}

test("only committed main-frame navigation in ordinary tabs reaches disk", t => {
  const file = temporaryHistory(t);
  const history = new BrowserHistory(file, 2);
  for (const tab of [undefined, { ...regular, login: true }, { ...regular, incognito: true },
    { ...regular, browserContextId: "private" }, { ...regular, kind: "editor" }]) {
    history.visit(tab, frame("https://private.example.com/"));
  }
  history.visit(regular, { ...frame("https://embedded.example.com/"), parentId: "parent" });
  history.visit(regular, { ...frame("chrome-error://chromewebdata/"), unreachableUrl: "https://failed.example.com/" });
  for (const host of ["one", "two", "three", "two"]) history.visit(regular, frame(`https://${host}.example.com/`));
  assert.deepEqual(new BrowserHistory(file, 2).entries(), ["https://two.example.com/", "https://three.example.com/"]);
  assert.doesNotMatch(readFileSync(file, "utf8"), /private|embedded|failed|one/);
  history.clear();
  assert.deepEqual(new BrowserHistory(file).entries(), []);
});

test("unreadable history fails independently of navigation and preserves the file", t => {
  const file = temporaryHistory(t);
  writeFileSync(file, "invalid");
  const history = new BrowserHistory(file);
  assert.deepEqual(history.entries(), []);
  history.visit(regular, frame("https://example.com/"));
  assert.equal(readFileSync(file, "utf8"), "invalid");
  history.clear();
  assert.deepEqual(new BrowserHistory(file).entries(), []);
});
