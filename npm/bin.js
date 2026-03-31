#!/usr/bin/env node
const { spawnSync } = require("child_process");
const path = require("path");

const bin = path.join(
  __dirname,
  "bin",
  process.platform === "win32" ? "supermodel.exe" : "supermodel"
);

const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
process.exit(result.status ?? 1);
