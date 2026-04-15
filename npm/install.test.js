// Tests for the Windows zip extraction logic in install.js.
// Uses Node's built-in test runner (node:test, available since Node 18).

"use strict";

const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { execSync } = require("child_process");
const { extractZip } = require("./install");

const isWindows = process.platform === "win32";
const isTarCmd = (c) => /tar(\.exe)?["'\s]/.test(c.split(" ")[0]);
const isPsCmd = (c) => c.includes("powershell");

// Make a fake extraction succeed by dropping a file into tmpDir so the
// post-extract verification (readdirSync(tmpDir).length > 0) passes.
const fakeExtract = (tmpDir) => () => {
  fs.writeFileSync(path.join(tmpDir, "supermodel.exe"), "fake");
};

test("extractZip succeeds when the first extractor produces files", () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "smt-ok-"));
  try {
    const commands = [];
    extractZip("/fake/archive.zip", tmpDir, (cmd) => {
      commands.push(cmd);
      fakeExtract(tmpDir)();
    });
    assert.equal(commands.length, 1, "stops at first successful extractor");
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test("extractZip falls back when first extractor throws", { skip: !isWindows }, () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "smt-throw-"));
  try {
    const commands = [];
    extractZip("/fake/archive.zip", tmpDir, (cmd) => {
      commands.push(cmd);
      if (isTarCmd(cmd)) throw new Error("tar unavailable");
      fakeExtract(tmpDir)();
    });
    assert.equal(commands.length, 2, "tries tar then PowerShell");
    assert.ok(isTarCmd(commands[0]), "first attempt is tar");
    assert.ok(isPsCmd(commands[1]), "second attempt is PowerShell");
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test("extractZip falls back when first extractor exits 0 but writes nothing", { skip: !isWindows }, () => {
  // Reproduces the GNU-tar-in-Git-Bash bug: tar prints "Cannot connect to C:"
  // but still exits 0 without extracting anything.
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "smt-silent-"));
  try {
    const commands = [];
    extractZip("/fake/archive.zip", tmpDir, (cmd) => {
      commands.push(cmd);
      if (isTarCmd(cmd)) return; // silent no-op "success"
      fakeExtract(tmpDir)();
    });
    assert.equal(commands.length, 2, "falls back when tar produced no files");
    assert.ok(isPsCmd(commands[1]), "fallback is PowerShell Expand-Archive");
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test("extractZip throws when every extractor fails", () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "smt-fail-"));
  try {
    assert.throws(() => {
      extractZip("/fake/archive.zip", tmpDir, () => {
        throw new Error("boom");
      });
    }, /boom/);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test("extractZip throws when every extractor silently produces nothing", () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "smt-empty-"));
  try {
    assert.throws(() => {
      extractZip("/fake/archive.zip", tmpDir, () => {
        // no-op: exit 0, no files — the exact silent-failure mode we must catch
      });
    });
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test("extractZip PowerShell fallback uses Expand-Archive with a retry loop", () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "smt-ps-"));
  try {
    const commands = [];
    try {
      extractZip("/fake/archive.zip", tmpDir, (cmd) => {
        commands.push(cmd);
        throw new Error("all fail");
      });
    } catch {}
    const psCmd = commands.find(isPsCmd);
    assert.ok(psCmd, "PowerShell command should be attempted");
    assert.ok(psCmd.includes("Expand-Archive"), "uses Expand-Archive");
    assert.ok(psCmd.includes("Start-Sleep"), "sleeps between retries");
    assert.match(psCmd, /-lt\s+\d+/, "has a retry counter");
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test("extractZip passes archive and tmpDir paths into every command", () => {
  const archive = "/tmp/test.zip";
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "smt-paths-"));
  try {
    const commands = [];
    try {
      extractZip(archive, tmpDir, (cmd) => {
        commands.push(cmd);
        throw new Error("fail all");
      });
    } catch {}
    for (const cmd of commands) {
      assert.ok(cmd.includes(archive), `archive path present: ${cmd}`);
      assert.ok(cmd.includes(tmpDir), `tmpDir path present: ${cmd}`);
    }
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});
