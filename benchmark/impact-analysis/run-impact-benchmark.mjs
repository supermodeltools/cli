#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const repoRoot = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const benchmarkRoot = path.join(repoRoot, 'benchmark', 'impact-analysis');
const defaultFixture = path.join(benchmarkRoot, 'fixtures', 'typescript-basic');
const defaultOutDir = path.join(repoRoot, 'target', 'impact-analysis-benchmark');
const publicApiRoot = path.resolve(
  process.env.SUPERMODEL_PUBLIC_API_REPO ||
  path.join(repoRoot, '..', 'supermodel-public-api')
);
const dataPlaneRoot = path.join(publicApiRoot, 'src', 'data-plane');

function parseArgs(argv) {
  const args = {
    fixture: defaultFixture,
    outDir: defaultOutDir,
    keepWorkdir: false,
    predictionFile: null,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--fixture') {
      args.fixture = path.resolve(argv[++i]);
    } else if (arg === '--out-dir') {
      args.outDir = path.resolve(argv[++i]);
    } else if (arg === '--prediction-file') {
      args.predictionFile = path.resolve(argv[++i]);
    } else if (arg === '--keep-workdir') {
      args.keepWorkdir = true;
    } else if (arg === '--help' || arg === '-h') {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }
  return args;
}

function printHelp() {
  console.log(`Usage: node benchmark/impact-analysis/run-impact-benchmark.mjs [options]

Options:
  --fixture <path>      Fixture directory with benchmark.json (default: fixtures/typescript-basic)
  --out-dir <path>      Directory for generated reports (default: target/impact-analysis-benchmark)
  --prediction-file     Override fixture prediction with a saved /v1/analysis/impact result
  --keep-workdir        Keep temporary mutated workdirs for inspection
`);
}

function resolveTscBin() {
  const candidates = [
    process.env.TSC_BIN,
    path.join(dataPlaneRoot, 'node_modules', '.bin', 'tsc'),
    path.join(repoRoot, 'node_modules', '.bin', 'tsc'),
  ].filter(Boolean);

  for (const candidate of candidates) {
    if (candidate && fs.existsSync(candidate)) return candidate;
  }
  return 'tsc';
}

function runCommand(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    shell: false,
  });
  return {
    command: [command, ...args].join(' '),
    cwd,
    status: result.status ?? 1,
    signal: result.signal,
    stdout: result.stdout || '',
    stderr: result.stderr || '',
    output: `${result.stdout || ''}${result.stderr || ''}`,
  };
}

function copyFixture(fixtureDir, tempRoot, label) {
  const dest = path.join(tempRoot, label);
  fs.cpSync(fixtureDir, dest, {
    recursive: true,
    filter: (src) => !src.includes(`${path.sep}dist${path.sep}`) && !src.endsWith(`${path.sep}dist`),
  });
  return dest;
}

function applyMutation(workdir, mutation) {
  const filePath = path.join(workdir, mutation.file);
  const before = fs.readFileSync(filePath, 'utf8');
  if (!before.includes(mutation.search)) {
    throw new Error(`Mutation "${mutation.name}" search text not found in ${mutation.file}`);
  }
  const after = before.replace(mutation.search, mutation.replace);
  fs.writeFileSync(filePath, after);
}

function runChecks(workdir, tscBin) {
  const typecheck = runCommand(tscBin, ['--project', 'tsconfig.json', '--noEmit'], workdir);
  const build = runCommand(tscBin, ['--project', 'tsconfig.json'], workdir);
  const tests = runCommand(process.execPath, ['--test', 'tests/cart.test.mjs'], workdir);
  return { typecheck, build, tests };
}

function extractFilesFromOutput(output, workdir) {
  const files = new Set();
  const escapedWorkdir = escapeRegExp(workdir.replaceAll(path.sep, '/'));
  const normalized = output.replaceAll(path.sep, '/');
  const patterns = [
    /(?:^|\n)(src\/[^:(\n]+\.ts)(?:\(\d+,\d+\)|:\d+:\d+)?:\s+error\b/g,
    new RegExp(`${escapedWorkdir}/(src/[^:(\\n]+\\.ts)(?:\\(\\d+,\\d+\\)|:\\d+:\\d+)?:\\s+error\\b`, 'g'),
    /(?:^|\n)#\s+at\s+.*?\(([^:\n]+):\d+:\d+\)/g,
  ];
  for (const pattern of patterns) {
    let match;
    while ((match = pattern.exec(normalized)) !== null) {
      const file = normalizeFixturePath(match[1], workdir);
      if (file.startsWith('src/') && file.endsWith('.ts')) files.add(file);
    }
  }
  return Array.from(files).sort();
}

function normalizeFixturePath(file, workdir) {
  let normalized = file.replaceAll(path.sep, '/');
  const normalizedWorkdir = workdir.replaceAll(path.sep, '/');
  if (normalized.startsWith(`${normalizedWorkdir}/`)) {
    normalized = normalized.slice(normalizedWorkdir.length + 1);
  }
  const distIdx = normalized.indexOf('dist/src/');
  if (distIdx >= 0) {
    normalized = `src/${normalized.slice(distIdx + 'dist/src/'.length)}`.replace(/\.js$/, '.ts');
  }
  return normalized;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function loadPrediction(fixtureDir, config, predictionFileOverride) {
  const predictionPath = predictionFileOverride || path.join(fixtureDir, config.predictionFile);
  const prediction = JSON.parse(fs.readFileSync(predictionPath, 'utf8'));
  const impact = prediction.impacts?.[0];
  if (!impact) throw new Error(`No impacts[0] in ${predictionPath}`);
  const files = (impact.affectedFiles || []).map((entry) => entry.file).filter(Boolean);
  return {
    path: predictionPath,
    target: impact.target,
    riskScore: impact.blastRadius?.riskScore || 'unknown',
    affectedFiles: files,
    raw: prediction,
  };
}

function computeMetrics(predictedFiles, actualFiles, riskScore) {
  const predicted = new Set(predictedFiles);
  const actual = new Set(actualFiles);
  const truePositiveFiles = predictedFiles.filter((file) => actual.has(file));
  const falsePositiveFiles = predictedFiles.filter((file) => !actual.has(file));
  const falseNegativeFiles = actualFiles.filter((file) => !predicted.has(file));
  const precision = predictedFiles.length === 0 ? (actualFiles.length === 0 ? 1 : 0) : truePositiveFiles.length / predictedFiles.length;
  const recall = actualFiles.length === 0 ? 1 : truePositiveFiles.length / actualFiles.length;
  return {
    precision,
    recall,
    topKRecall: {
      1: recallAtK(predictedFiles, actual, 1),
      3: recallAtK(predictedFiles, actual, 3),
      5: recallAtK(predictedFiles, actual, 5),
    },
    riskCalibration: calibrateRisk(riskScore, actualFiles.length),
    truePositiveFiles,
    falsePositiveFiles,
    falseNegativeFiles,
  };
}

function recallAtK(predictedFiles, actualSet, k) {
  if (actualSet.size === 0) return 1;
  let hits = 0;
  for (const file of predictedFiles.slice(0, k)) {
    if (actualSet.has(file)) hits += 1;
  }
  return hits / actualSet.size;
}

function calibrateRisk(predictedRisk, actualFileCount) {
  const actualRisk = actualRiskFromFileCount(actualFileCount);
  return {
    predictedRisk,
    actualRisk,
    matches: predictedRisk === actualRisk,
    actualFileCount,
    rubric: {
      none: '0 broken files',
      low: '1 broken file',
      medium: '2-4 broken files',
      high: '5-9 broken files',
      critical: '10+ broken files',
    },
  };
}

function actualRiskFromFileCount(count) {
  if (count === 0) return 'none';
  if (count === 1) return 'low';
  if (count <= 4) return 'medium';
  if (count <= 9) return 'high';
  return 'critical';
}

function writeMarkdownReport(report, outPath) {
  const mutationRows = report.mutations.map((mutation) => {
    const metrics = mutation.metrics;
    return [
      `| ${mutation.name} | ${formatNumber(metrics.precision)} | ${formatNumber(metrics.recall)} | ${formatNumber(metrics.topKRecall[3])} | ${metrics.riskCalibration.predictedRisk} | ${metrics.riskCalibration.actualRisk} | ${metrics.truePositiveFiles.length} | ${metrics.falsePositiveFiles.length} | ${metrics.falseNegativeFiles.length} |`,
      '',
      `Actual broken files: ${formatList(mutation.actualFiles)}`,
      '',
      `False positives: ${formatList(metrics.falsePositiveFiles)}`,
      '',
      `False negatives: ${formatList(metrics.falseNegativeFiles)}`,
    ].join('\n');
  }).join('\n\n');

  const markdown = `# Impact Analysis Benchmark Report

- Fixture: \`${report.fixture.name}\`
- Target: \`${report.prediction.target.file}:${report.prediction.target.name}\`
- Prediction file: \`${path.relative(repoRoot, report.prediction.path)}\`
- Generated: ${report.generatedAt}

| Mutation | Precision | Recall | Recall@3 | Predicted risk | Actual risk | TP | FP | FN |
|---|---:|---:|---:|---|---|---:|---:|---:|
${report.mutations.map((mutation) => {
    const metrics = mutation.metrics;
    return `| ${mutation.name} | ${formatNumber(metrics.precision)} | ${formatNumber(metrics.recall)} | ${formatNumber(metrics.topKRecall[3])} | ${metrics.riskCalibration.predictedRisk} | ${metrics.riskCalibration.actualRisk} | ${metrics.truePositiveFiles.length} | ${metrics.falsePositiveFiles.length} | ${metrics.falseNegativeFiles.length} |`;
  }).join('\n')}

## Details

${mutationRows}

## Interpretation

Precision below 1.0 means the impact prediction included files that did not surface new typecheck/test failures for this mutation. Recall below 1.0 means the mutation broke files that were not predicted. This benchmark only measures the selected mutation shape; it does not prove all possible semantic blast radius.
`;
  fs.writeFileSync(outPath, markdown);
}

function formatNumber(value) {
  return Number.isFinite(value) ? value.toFixed(3) : String(value);
}

function formatList(values) {
  return values.length ? values.map((value) => `\`${value}\``).join(', ') : '_none_';
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const fixtureDir = args.fixture;
  const configPath = path.join(fixtureDir, 'benchmark.json');
  const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));
  const prediction = loadPrediction(fixtureDir, config, args.predictionFile);
  const tscBin = resolveTscBin();
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'supermodel-impact-benchmark-'));
  fs.mkdirSync(args.outDir, { recursive: true });

  const report = {
    generatedAt: new Date().toISOString(),
    repoRoot,
    fixture: {
      name: config.name,
      description: config.description,
      path: fixtureDir,
    },
    tscBin,
    prediction: {
      path: prediction.path,
      target: prediction.target,
      riskScore: prediction.riskScore,
      affectedFiles: prediction.affectedFiles,
    },
    baseline: null,
    mutations: [],
    tempRoot: args.keepWorkdir ? tempRoot : undefined,
  };

  try {
    const baselineDir = copyFixture(fixtureDir, tempRoot, 'baseline');
    const baselineChecks = runChecks(baselineDir, tscBin);
    report.baseline = summarizeChecks(baselineChecks, baselineDir);

    const baselineFailed = Object.values(baselineChecks).some((check) => check.status !== 0);
    if (baselineFailed) {
      throw new Error(`Baseline checks failed; refusing to score mutations. See ${args.outDir}`);
    }

    for (const mutation of config.mutations) {
      const mutationDir = copyFixture(fixtureDir, tempRoot, mutation.name);
      applyMutation(mutationDir, mutation);
      const checks = runChecks(mutationDir, tscBin);
      const output = Object.values(checks).map((check) => check.output).join('\n');
      const actualFiles = extractFilesFromOutput(output, mutationDir);
      if (checks.tests.status !== 0 && actualFiles.length === 0) {
        throw new Error(`Mutation "${mutation.name}" failed tests, but no source files were attributed from the output.`);
      }
      const metrics = computeMetrics(prediction.affectedFiles, actualFiles, prediction.riskScore);
      report.mutations.push({
        name: mutation.name,
        description: mutation.description,
        changedFile: mutation.file,
        checks: summarizeChecks(checks, mutationDir),
        actualFiles,
        expectedActualFiles: config.expectedActualFiles || [],
        metrics,
      });
    }
  } finally {
    const jsonPath = path.join(args.outDir, 'impact-benchmark-report.json');
    const mdPath = path.join(args.outDir, 'impact-benchmark-report.md');
    fs.writeFileSync(jsonPath, `${JSON.stringify(report, null, 2)}\n`);
    writeMarkdownReport(report, mdPath);
    console.log(`Wrote ${path.relative(repoRoot, jsonPath)}`);
    console.log(`Wrote ${path.relative(repoRoot, mdPath)}`);
    if (!args.keepWorkdir) fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function summarizeChecks(checks, workdir) {
  return Object.fromEntries(Object.entries(checks).map(([name, check]) => [
    name,
    {
      command: check.command,
      status: check.status,
      signal: check.signal,
      implicatedFiles: extractFilesFromOutput(check.output, workdir),
      stdout: check.stdout,
      stderr: check.stderr,
    },
  ]));
}

main();
