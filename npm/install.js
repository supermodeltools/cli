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
    file.on("finish", () => file.close(cb));
  }).on("error", (err) => fail(err.message));
}

// extractZip extracts a .zip archive into tmpDir.
// Tries native tar first (Windows 10+); falls back to PowerShell Expand-Archive
// with a retry loop to handle transient Antivirus file locks.
// Accepts an optional execFn for testing (defaults to execSync).
function extractZip(archive, tmpDir, execFn) {
  const exec = execFn || execSync;
  try {
    exec(`tar -xf "${archive}" -C "${tmpDir}"`);
  } catch {
    const psCommand =
      `$RetryCount = 0; while ($RetryCount -lt 10) { try { Expand-Archive` +
      ` -Force -Path '${archive}' -DestinationPath '${tmpDir}'; break }` +
      ` catch { Start-Sleep -Seconds 1; $RetryCount++ } }`;
    exec(`powershell -NoProfile -Command "${psCommand}"`);
  }
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
