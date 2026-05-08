#!/usr/bin/env node
import {randomUUID} from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const repoRoot = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const defaultOutDir = path.join(repoRoot, 'target', 'impact-api-smoke');
const publicApiRoot = path.resolve(
  process.env.SUPERMODEL_PUBLIC_API_REPO ||
  path.join(repoRoot, '..', 'supermodel-public-api')
);

function parseArgs(argv) {
  const args = {
    baseUrl: process.env.API_BASE_URL || process.env.SUPERMODEL_API_URL || 'http://127.0.0.1:8080',
    outDir: defaultOutDir,
    repoDir: publicApiRoot,
    zip: null,
    diff: null,
    targets: 'src/data-plane/src/services/job-worker.service.ts',
    timeoutMs: 900000,
    maxAttempts: 60,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--base-url') args.baseUrl = argv[++i];
    else if (arg === '--out-dir') args.outDir = path.resolve(argv[++i]);
    else if (arg === '--repo-dir') args.repoDir = path.resolve(argv[++i]);
    else if (arg === '--zip') args.zip = path.resolve(argv[++i]);
    else if (arg === '--diff') args.diff = path.resolve(argv[++i]);
    else if (arg === '--targets') args.targets = argv[++i];
    else if (arg === '--timeout-ms') args.timeoutMs = Number(argv[++i]);
    else if (arg === '--max-attempts') args.maxAttempts = Number(argv[++i]);
    else if (arg === '--help' || arg === '-h') {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }

  return args;
}

function printHelp() {
  console.log(`Usage: node benchmark/agent-impact/run-impact-api-smoke.mjs [options]

Environment:
  SUPERMODEL_API_KEY            Required. Sent as X-Api-Key, never logged.
  API_BASE_URL                  Optional. Default: http://127.0.0.1:8080
  SUPERMODEL_PUBLIC_API_REPO    Public API checkout to archive when --repo-dir is omitted

Options:
  --base-url <url>              API base URL
  --repo-dir <path>             Git repo to archive when --zip is omitted
  --zip <path>                  Prebuilt repository zip
  --diff <path>                 Optional unified diff file
  --targets <targets>           Comma-separated file or file:function targets
  --out-dir <path>              Output directory
  --timeout-ms <n>              Total polling timeout
  --max-attempts <n>            Max polling attempts
`);
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd || repoRoot,
    encoding: 'utf8',
    shell: false,
    timeout: options.timeoutMs,
    maxBuffer: 100 * 1024 * 1024,
  });
  return {
    command: [command, ...args].join(' '),
    status: result.status ?? 1,
    stdout: result.stdout || '',
    stderr: result.stderr || '',
    output: `${result.stdout || ''}${result.stderr || ''}`,
  };
}

function mustRun(command, args, options = {}) {
  const result = run(command, args, options);
  if (result.status !== 0) {
    throw new Error(`Command failed: ${result.command}\n${result.output}`);
  }
  return result;
}

function createArchive(repoDir, outDir) {
  const zipPath = path.join(outDir, 'repo.zip');
  mustRun('git', ['archive', '--format=zip', 'HEAD', '-o', zipPath], {cwd: repoDir, timeoutMs: 120000});
  return zipPath;
}

async function postImpact(args, apiKey, zipPath, idempotencyKey) {
  const url = new URL('/v1/analysis/impact', args.baseUrl.replace(/\/+$/, ''));
  if (args.targets) url.searchParams.set('targets', args.targets);
  url.searchParams.set('includeValidationFiles', 'true');

  const form = new FormData();
  const zipBytes = fs.readFileSync(zipPath);
  form.append('file', new Blob([zipBytes], {type: 'application/zip'}), path.basename(zipPath));
  if (args.diff) {
    const diffBytes = fs.readFileSync(args.diff);
    form.append('diff', new Blob([diffBytes], {type: 'text/plain'}), path.basename(args.diff));
  }

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Idempotency-Key': idempotencyKey,
      'X-Api-Key': apiKey,
    },
    body: form,
  });

  const text = await response.text();
  let body;
  try {
    body = text ? JSON.parse(text) : {};
  } catch {
    body = {raw: text};
  }

  if (!response.ok && response.status !== 202) {
    throw new Error(`Impact API returned HTTP ${response.status}: ${JSON.stringify(body).slice(0, 1000)}`);
  }

  return {statusCode: response.status, retryAfterHeader: response.headers.get('retry-after'), body};
}

function validateCompleted(result) {
  const errors = [];
  if (!result?.metadata) errors.push('missing result.metadata');
  if (!Array.isArray(result?.impacts)) errors.push('missing result.impacts[]');
  if (!result?.globalMetrics) errors.push('missing result.globalMetrics');
  for (const impact of result?.impacts || []) {
    if (!impact.target?.file) errors.push('impact missing target.file');
    if (!impact.blastRadius?.riskScore) errors.push(`impact ${impact.target?.file || '<unknown>'} missing blastRadius.riskScore`);
    if (impact.validationFiles !== undefined && !Array.isArray(impact.validationFiles)) {
      errors.push(`impact ${impact.target?.file || '<unknown>'} validationFiles is not an array`);
    }
    if (impact.primaryValidationFiles !== undefined && !Array.isArray(impact.primaryValidationFiles)) {
      errors.push(`impact ${impact.target?.file || '<unknown>'} primaryValidationFiles is not an array`);
    }
  }
  return errors;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const apiKey = process.env.SUPERMODEL_API_KEY;
  if (!apiKey) {
    throw new Error('SUPERMODEL_API_KEY is required in the environment');
  }

  const runId = new Date().toISOString().replace(/[:.]/g, '-');
  const outDir = path.join(args.outDir, runId);
  fs.mkdirSync(outDir, {recursive: true});

  const zipPath = args.zip || createArchive(args.repoDir, outDir);
  const idempotencyKey = `impact-smoke-${os.hostname()}-${randomUUID()}`;
  const startedAt = Date.now();
  let lastResponse = null;

  for (let attempt = 1; attempt <= args.maxAttempts; attempt += 1) {
    if (Date.now() - startedAt > args.timeoutMs) {
      throw new Error(`Timed out after ${args.timeoutMs}ms waiting for impact result`);
    }

    lastResponse = await postImpact(args, apiKey, zipPath, idempotencyKey);
    const body = lastResponse.body;
    fs.writeFileSync(path.join(outDir, `response-${String(attempt).padStart(2, '0')}.json`), `${JSON.stringify(body, null, 2)}\n`);

    const status = body.status || `http-${lastResponse.statusCode}`;
    console.log(`[impact-api-smoke] attempt ${attempt}/${args.maxAttempts}: ${status}`);

    if (body.status === 'completed') {
      const errors = validateCompleted(body.result);
      if (errors.length > 0) {
        throw new Error(`Impact response validation failed: ${errors.join(', ')}`);
      }
      const summary = {
        baseUrl: args.baseUrl,
        targets: args.targets,
        diffProvided: Boolean(args.diff),
        zipPath,
        idempotencyKey,
        attempts: attempt,
        outputDir: outDir,
        metadata: body.result.metadata,
        impacts: body.result.impacts.map(impact => ({
          target: impact.target,
          affectedFiles: impact.affectedFiles?.length || 0,
          primaryValidationFiles: impact.primaryValidationFiles?.length || 0,
          validationFiles: impact.validationFiles?.length || 0,
          riskScore: impact.blastRadius?.riskScore,
        })),
      };
      fs.writeFileSync(path.join(outDir, 'summary.json'), `${JSON.stringify(summary, null, 2)}\n`);
      console.log(`[impact-api-smoke] completed; wrote ${path.relative(repoRoot, path.join(outDir, 'summary.json'))}`);
      return;
    }

    if (body.status === 'failed') {
      throw new Error(`Impact job failed: ${body.error || 'unknown error'}`);
    }

    const retryAfterSeconds = Number(body.retryAfter || lastResponse.retryAfterHeader || 10);
    await new Promise(resolve => setTimeout(resolve, retryAfterSeconds * 1000));
  }

  throw new Error(`Impact job did not complete after ${args.maxAttempts} attempts`);
}

main().catch(error => {
  console.error(error.message);
  process.exit(1);
});
