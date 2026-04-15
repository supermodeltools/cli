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

// createTestZip builds a real .zip containing a single file named "supermodel.exe"
// using the system zip/tar command. Skips on platforms where neither is available.
function createTestZip(t) {
  const src = fs.mkdtempSync(path.join(os.tmpdir(), "supermodel-test-src-"));
  const binary = path.join(src, "supermodel.exe");
  fs.writeFileSync(binary, "fake binary");

  const archive = path.join(os.tmpdir(), `supermodel-test-${process.pid}.zip`);
  try {
    // Use system zip or tar to build the archive.
    try {
      execSync(`zip -j "${archive}" "${binary}"`, { stdio: "pipe" });
    } catch {
      execSync(`tar -cf "${archive}" --format=zip -C "${src}" supermodel.exe`, { stdio: "pipe" });
    }
  } catch {
    fs.rmSync(src, { recursive: true, force: true });
    return null; // zip tooling not available — caller should skip
  }
  fs.rmSync(src, { recursive: true, force: true });
  return archive;
}

test("extractZip extracts via tar when tar succeeds", () => {
  const archive = createTestZip();
  if (!archive) {
    // Skip gracefully if zip tooling unavailable (e.g. minimal CI image).
    console.log("  skipped: zip tooling not available");
    return;
  }
  try {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "supermodel-test-out-"));
    try {
      let called = null;
      extractZip(archive, tmpDir, (cmd) => {
        called = cmd;
        // Only simulate tar; let the actual extraction happen via real execSync
        // if this is the tar command.
        if (cmd.startsWith("tar")) {
          execSync(cmd, { stdio: "pipe" });
        } else {
          throw new Error("should not reach PowerShell");
        }
      });
      assert.ok(called.startsWith("tar"), "should have called tar first");
      const extracted = fs.readdirSync(tmpDir);
      assert.ok(extracted.length > 0, "tmpDir should contain extracted files");
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  } finally {
    fs.unlinkSync(archive);
  }
});

test("extractZip falls back to PowerShell when tar fails", () => {
  const commands = [];
  // Simulate tar failing; PowerShell "succeeds" (no-op).
  extractZip("/fake/archive.zip", "/fake/tmpdir", (cmd) => {
    commands.push(cmd);
    if (cmd.startsWith("tar")) throw new Error("tar not available");
    // PowerShell call — just record it, don't execute.
  });

  assert.equal(commands.length, 2, "should have attempted tar then PowerShell");
  assert.ok(commands[0].startsWith("tar"), "first call should be tar");
  assert.ok(commands[1].includes("powershell"), "second call should be PowerShell");
  assert.ok(commands[1].includes("Expand-Archive"), "PowerShell command should use Expand-Archive");
});

test("extractZip PowerShell fallback includes retry loop", () => {
  const commands = [];
  extractZip("/fake/archive.zip", "/fake/tmpdir", (cmd) => {
    commands.push(cmd);
    if (cmd.startsWith("tar")) throw new Error("tar not available");
  });

  const psCmd = commands.find((c) => c.includes("powershell"));
  assert.ok(psCmd, "PowerShell command should be present");
  assert.ok(psCmd.includes("$RetryCount"), "should include retry counter");
  assert.ok(psCmd.includes("Start-Sleep"), "should include sleep between retries");
  assert.ok(psCmd.includes("-lt 10"), "should retry up to 10 times");
});

test("extractZip uses tar when both succeed — tar wins", () => {
  const commands = [];
  extractZip("/fake/archive.zip", "/fake/tmpdir", (cmd) => {
    commands.push(cmd);
    // Both would succeed; tar is tried first and doesn't throw.
  });

  assert.equal(commands.length, 1, "should only call tar when it succeeds");
  assert.ok(commands[0].startsWith("tar"), "the single call should be tar");
});

test("extractZip passes archive and tmpDir paths into tar command", () => {
  const archive = "/tmp/test.zip";
  const tmpDir = "/tmp/extract-dir";
  let tarCmd = null;
  extractZip(archive, tmpDir, (cmd) => {
    tarCmd = cmd;
  });

  assert.ok(tarCmd.includes(archive), "tar command should include archive path");
  assert.ok(tarCmd.includes(tmpDir), "tar command should include tmpDir path");
});

test("extractZip passes archive and tmpDir paths into PowerShell fallback", () => {
  const archive = "/tmp/test.zip";
  const tmpDir = "/tmp/extract-dir";
  const commands = [];
  extractZip(archive, tmpDir, (cmd) => {
    commands.push(cmd);
    if (cmd.startsWith("tar")) throw new Error("tar failed");
  });

  const psCmd = commands[1];
  assert.ok(psCmd.includes(archive), "PowerShell command should include archive path");
  assert.ok(psCmd.includes(tmpDir), "PowerShell command should include tmpDir path");
});
