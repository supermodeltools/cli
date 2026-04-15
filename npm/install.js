#!/usr/bin/env node
// postinstall: downloads the correct platform binary from GitHub Releases.

"use strict";

const { execSync } = require("child_process");
const fs = require("fs");
const https = require("https");
const os = require("os");
const path = require("path");

const REPO = "supermodeltools/cli";
const BIN_DIR = path.join(__dirname, "bin");
const BIN_PATH = path.join(BIN_DIR, process.platform === "win32" ? "supermodel.exe" : "supermodel");

const PLATFORM_MAP = {
  darwin: "Darwin",
  linux: "Linux",
  win32: "Windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function fail(msg) {
  console.error(`\n[supermodel] ${msg}`);
  process.exit(1);
}

function download(url, dest, cb) {
  const file = fs.createWriteStream(dest);
  https.get(url, (res) => {
    if (res.statusCode === 302 || res.statusCode === 301) {
      return download(res.headers.location, dest, cb);
    }
    if (res.statusCode !== 200) {
      return fail(`HTTP ${res.statusCode} downloading ${url}`);
    }
    res.pipe(file);
    // Wait for 'close' (not just 'finish'): on Windows the fd is not actually
    // released until close completes, and subsequent extractors (PowerShell
    // Expand-Archive in particular) will fail with a file-in-use error.
    file.on("finish", () => file.close());
    file.on("close", () => cb());
  }).on("error", (err) => fail(err.message));
}

// extractZip extracts a .zip archive into tmpDir.
//
// On Windows, PATH under Git Bash (npm's postinstall shell) resolves `tar`
// to GNU tar, which parses `C:\...` as `host:path` (rsh syntax), prints
// "Cannot connect to C: resolve failed", and exits 0 with nothing extracted.
// We explicitly call the native Windows bsdtar at %SystemRoot%\System32\tar.exe
// (present on Windows 10+), which handles both Windows paths and the zip
// format. On older Windows we fall back to PowerShell Expand-Archive with
// retries for Defender/indexer file locks.
//
// After extraction we always verify tmpDir is non-empty — some extractors
// (notably GNU tar in the failure mode above) exit 0 without writing anything.
// Accepts an optional execFn for testing (defaults to execSync).
function extractZip(archive, tmpDir, execFn) {
  const exec = execFn || execSync;
  const attempts = [];

  const bsdtar = path.join(process.env.SystemRoot || "C:\\Windows", "System32", "tar.exe");
  if (process.platform === "win32" && fs.existsSync(bsdtar)) {
    attempts.push({ cmd: `"${bsdtar}" -xf "${archive}" -C "${tmpDir}"`, label: "bsdtar" });
  }
  // PowerShell fallback. Retries handle transient locks from Windows Defender
  // / Search Indexer on the freshly-written zip. Throws if all retries fail.
  const psCommand =
    `$ErrorActionPreference='Stop'; ` +
    `$lastErr = $null; ` +
    `for ($i=0; $i -lt 20; $i++) { ` +
    `  try { Expand-Archive -Force -Path '${archive}' -DestinationPath '${tmpDir}'; exit 0 } ` +
    `  catch { $lastErr = $_; Start-Sleep -Seconds 1 } ` +
    `}; ` +
    `throw $lastErr`;
  attempts.push({ cmd: `powershell -NoProfile -Command "${psCommand}"`, label: "powershell" });

  let lastErr = null;
  for (const { cmd } of attempts) {
    try {
      exec(cmd);
      let entries = [];
      try { entries = fs.readdirSync(tmpDir); } catch { /* missing dir => failure */ }
      if (entries.length > 0) return;
    } catch (e) {
      lastErr = e;
    }
  }
  throw lastErr || new Error(`[supermodel] extraction produced no files in ${tmpDir}`);
}

if (require.main === module) {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[os.arch()];

  if (!platform) fail(`Unsupported platform: ${process.platform}`);
  if (!arch) fail(`Unsupported architecture: ${os.arch()}`);

  const ext = process.platform === "win32" ? "zip" : "tar.gz";
  const archive = `supermodel_${platform}_${arch}.${ext}`;
  const tag = `v${require("./package.json").version}`;
  const url = `https://github.com/${REPO}/releases/download/${tag}/${archive}`;
  const tmpArchive = path.join(os.tmpdir(), archive);

  console.log(`[supermodel] Downloading ${archive} from GitHub Releases...`);
  fs.mkdirSync(BIN_DIR, { recursive: true });

  download(url, tmpArchive, () => {
    if (ext === "tar.gz") {
      execSync(`tar -xzf "${tmpArchive}" -C "${BIN_DIR}" supermodel`);
    } else {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "supermodel-extract-"));
      try {
        extractZip(tmpArchive, tmpDir);
        fs.copyFileSync(path.join(tmpDir, "supermodel.exe"), BIN_PATH);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    }
    if (process.platform !== "win32") fs.chmodSync(BIN_PATH, 0o755);
    fs.unlinkSync(tmpArchive);
    console.log(`[supermodel] Installed to ${BIN_PATH}`);
  });
}

module.exports = { extractZip };
