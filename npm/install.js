#!/usr/bin/env node
// postinstall: downloads the correct platform binary from GitHub Releases.

const { execSync } = require("child_process");
const fs = require("fs");
const https = require("https");
const os = require("os");
const path = require("path");
const { createGunzip } = require("zlib");

const REPO = "supermodeltools/cli";
const BIN_DIR = path.join(__dirname, "bin");
const BIN_PATH = path.join(BIN_DIR, process.platform === "win32" ? "supermodel.exe" : "supermodel");

const VERSION = require("./package.json").version;

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

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[os.arch()];

if (!platform) fail(`Unsupported platform: ${process.platform}`);
if (!arch) fail(`Unsupported architecture: ${os.arch()}`);

const ext = process.platform === "win32" ? "zip" : "tar.gz";
const archive = `supermodel_${platform}_${arch}.${ext}`;
const tag = `v${VERSION}`;
const url = `https://github.com/${REPO}/releases/download/${tag}/${archive}`;

console.log(`[supermodel] Downloading ${archive} from GitHub Releases...`);

fs.mkdirSync(BIN_DIR, { recursive: true });

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

const tmpArchive = path.join(os.tmpdir(), archive);

download(url, tmpArchive, () => {
  if (ext === "tar.gz") {
    execSync(`tar -xzf "${tmpArchive}" -C "${BIN_DIR}" supermodel`);
  } else {
    // Windows 10+ natively supports tar. Using tar avoids Antivirus file lock
    // crashes commonly seen with PowerShell's Expand-Archive cmdlet.
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "supermodel-extract-"));
    try {
      execSync(`tar -xf "${tmpArchive}" -C "${tmpDir}"`);
    } catch {
      const psCommand = `$RetryCount = 0; while ($RetryCount -lt 10) { try { Expand-Archive -Force -Path '${tmpArchive}' -DestinationPath '${tmpDir}'; break } catch { Start-Sleep -Seconds 1; $RetryCount++ } }`;
      execSync(
        `powershell -NoProfile -Command "${psCommand}"`,
      );
    }
    fs.copyFileSync(path.join(tmpDir, "supermodel.exe"), BIN_PATH);
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
  if (process.platform !== "win32") fs.chmodSync(BIN_PATH, 0o755);
  fs.unlinkSync(tmpArchive);
  console.log(`[supermodel] Installed to ${BIN_PATH}`);
});
