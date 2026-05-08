#!/usr/bin/env node
import {spawnSync} from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import {fileURLToPath} from 'node:url';

const repoRoot = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const benchmarkRoot = path.join(repoRoot, 'benchmark', 'agent-impact');
const defaultManifest = path.join(benchmarkRoot, 'post-cutoff-prs.json');
const defaultOutDir = path.join(repoRoot, 'target', 'post-cutoff-pr-replay');

function parseArgs(argv) {
  const args = {
    manifest: defaultManifest,
    outDir: defaultOutDir,
    image: 'supermodel-agent-impact-go:local',
    model: 'gpt-5.5',
    codexHome: path.join(os.homedir(), '.codex'),
    cases: null,
    arms: ['control', 'impact'],
    timeoutSeconds: 1200,
    preflightOnly: false,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--manifest') args.manifest = path.resolve(argv[++i]);
    else if (arg === '--out-dir') args.outDir = path.resolve(argv[++i]);
    else if (arg === '--image') args.image = argv[++i];
    else if (arg === '--model') args.model = argv[++i];
    else if (arg === '--codex-home') args.codexHome = path.resolve(argv[++i].replace(/^~(?=$|\/)/, os.homedir()));
    else if (arg === '--cases') args.cases = argv[++i].split(',').map(value => value.trim()).filter(Boolean);
    else if (arg === '--arms') args.arms = argv[++i].split(',').map(value => value.trim()).filter(Boolean);
    else if (arg === '--timeout-seconds') args.timeoutSeconds = Number(argv[++i]);
    else if (arg === '--preflight-only') args.preflightOnly = true;
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
  console.log(`Usage: node benchmark/agent-impact/run-post-cutoff-pr-replay.mjs [options]

Options:
  --manifest <path>          Manifest path (default: benchmark/agent-impact/post-cutoff-prs.json)
  --out-dir <path>           Output directory (default: target/post-cutoff-pr-replay)
  --image <name>             Docker image with codex/go installed (default: supermodel-agent-impact-go:local)
  --model <name>             Agent model (default: gpt-5.5)
  --codex-home <path>        Codex auth/config directory copied per run (default: ~/.codex)
  --cases <a,b,c>            Comma-separated candidate ids
  --arms <control,impact>    Comma-separated arms to run
  --timeout-seconds <n>      Agent timeout override
  --preflight-only           Prepare failing/reference states and skip agent calls
`);
}

function run(command, args, options = {}) {
  const startedAt = Date.now();
  const result = spawnSync(command, args, {
    cwd: options.cwd || repoRoot,
    encoding: 'utf8',
    shell: false,
    timeout: options.timeoutMs,
    env: {...process.env, ...(options.env || {})},
    maxBuffer: 100 * 1024 * 1024,
  });

  return {
    command: [command, ...args].join(' '),
    cwd: options.cwd || repoRoot,
    status: result.status ?? 1,
    signal: result.signal,
    durationMs: Date.now() - startedAt,
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

function dockerRun(image, mounts, shellScript, logPath, timeoutMs) {
  const args = ['run', '--rm'];
  for (const [host, container] of mounts) {
    args.push('-v', `${host}:${container}`);
  }
  args.push(image, 'bash', '-lc', shellScript);
  const result = run('docker', args, {timeoutMs});
  if (logPath) fs.writeFileSync(logPath, result.output);
  return result;
}

function runRepoCommands(image, repoDir, commands, logPath, timeoutMs) {
  const script = [
    'set -euo pipefail',
    'cd /workspace/repo',
    ...commands,
  ].join('\n');
  return dockerRun(image, [[repoDir, '/workspace/repo']], script, logPath, timeoutMs);
}

function cloneCandidate(candidate, repoDir) {
  fs.rmSync(repoDir, {recursive: true, force: true});
  fs.mkdirSync(path.dirname(repoDir), {recursive: true});
  mustRun('git', ['clone', '--filter=blob:none', '--no-tags', `https://github.com/${candidate.repo}.git`, repoDir], {
    timeoutMs: 20 * 60 * 1000,
  });
  mustRun('git', ['checkout', '--detach', candidate.baseSha], {cwd: repoDir});
  mustRun('git', ['fetch', '--depth', '1', 'origin', candidate.mergeCommitSha], {cwd: repoDir, timeoutMs: 10 * 60 * 1000});
  mustRun('git', ['config', 'user.email', 'post-cutoff-pr-replay@example.invalid'], {cwd: repoDir});
  mustRun('git', ['config', 'user.name', 'Post-Cutoff PR Replay'], {cwd: repoDir});
}

function applyFilesFromReference(repoDir, candidate, files) {
  mustRun('git', ['checkout', candidate.mergeCommitSha, '--', ...files], {cwd: repoDir});
}

function commitBaseline(repoDir) {
  mustRun('git', ['add', '.'], {cwd: repoDir});
  mustRun('git', ['commit', '--no-verify', '-m', 'benchmark regression baseline'], {cwd: repoDir});
}

function prepareCodexHome(source, dest) {
  fs.rmSync(dest, {recursive: true, force: true});
  fs.mkdirSync(dest, {recursive: true});
  if (!fs.existsSync(source)) return;

  fs.cpSync(source, dest, {
    recursive: true,
    filter: sourcePath => {
      const normalized = sourcePath.replaceAll(path.sep, '/');
      return !normalized.includes('/sessions/') &&
        !normalized.includes('/archived_sessions/') &&
        !normalized.includes('/log/') &&
        !normalized.endsWith('/history.jsonl');
    },
  });
}

function writeImpactFiles(runDir, repoDir, candidate) {
  const context = {
    generatedBy: 'post-cutoff-pr-replay-oracle-file-context',
    warning: 'This is an oracle upper-bound context derived from the PR reference production files. It is not proof that Supermodel generated these files unaided.',
    mode: candidate.replay.impactContextMode,
    pr: {
      repo: candidate.repo,
      number: candidate.number,
      title: candidate.title,
      mergedAt: candidate.mergedAt,
    },
    confidenceTiers: {
      likelyAffectedFiles: candidate.replay.productionFiles,
      possibleContextFiles: candidate.replay.testFiles,
      architecturalContextFiles: [],
    },
  };

  fs.writeFileSync(path.join(runDir, 'impact-analysis.json'), `${JSON.stringify(context, null, 2)}\n`);
  fs.writeFileSync(path.join(runDir, 'IMPACT_ANALYSIS.md'), `# Impact Analysis

Mode: \`${context.mode}\`

This context is an oracle upper bound derived from the post-cutoff PR reference production files. It is useful for asking whether a strong file-ranking packet can help the agent, but it is not yet proof that Supermodel can generate the same packet.

## Likely affected production files

${formatList(context.confidenceTiers.likelyAffectedFiles)}

## Regression/test context files

${formatList(context.confidenceTiers.possibleContextFiles)}
`);
  fs.copyFileSync(path.join(runDir, 'impact-analysis.json'), path.join(repoDir, 'impact-analysis.json'));
  fs.copyFileSync(path.join(runDir, 'IMPACT_ANALYSIS.md'), path.join(repoDir, 'IMPACT_ANALYSIS.md'));
}

function writePrompt(runDir, candidate, arm) {
  const verifier = candidate.replay.verifier.map(command => `  - ${command}`).join('\n');
  const impactText = arm === 'impact' ?
    'Oracle file-ranking context is available in IMPACT_ANALYSIS.md and impact-analysis.json. Treat it as a starting map, then verify in code.' :
    'No impact-analysis context is available. Find the affected production files yourself.';

  const prompt = `You are repairing a real post-cutoff PR replay benchmark.

Repository: ${candidate.repo}
PR: #${candidate.number} ${candidate.title}
Merged at: ${candidate.mergedAt}

The checkout is at the PR base commit with the PR regression test files applied, but the production fix has been withheld.

Rules:
- Make the verifier pass by fixing production behavior.
- Do not remove, weaken, or rewrite the regression test to hide the failure.
- Do not revert the benchmark regression baseline commit.
- Do not fetch, inspect, checkout, or apply the PR merge commit or patch.
- Keep the repair as small as possible.

Verifier:
${verifier}

${impactText}

When finished, leave the repository in a passing state.`;

  fs.writeFileSync(path.join(runDir, 'prompt.md'), prompt);
}

function prepareAgentIoDir(runDir, arm) {
  const agentIoDir = path.join(runDir, 'agent-io');
  fs.rmSync(agentIoDir, {recursive: true, force: true});
  fs.mkdirSync(agentIoDir, {recursive: true});
  fs.copyFileSync(path.join(runDir, 'prompt.md'), path.join(agentIoDir, 'prompt.md'));
  if (arm === 'impact') {
    fs.copyFileSync(path.join(runDir, 'impact-analysis.json'), path.join(agentIoDir, 'impact-analysis.json'));
    fs.copyFileSync(path.join(runDir, 'IMPACT_ANALYSIS.md'), path.join(agentIoDir, 'IMPACT_ANALYSIS.md'));
  }
  return agentIoDir;
}

function runAgent(image, repoDir, runDir, codexHomeDir, model, timeoutSeconds) {
  const agentIoDir = prepareAgentIoDir(runDir, path.basename(runDir));
  const script = [
    'set -euo pipefail',
    'cd /workspace/repo',
    `timeout ${timeoutSeconds}s codex exec --json --model ${shellQuote(model)} --dangerously-bypass-approvals-and-sandbox --skip-git-repo-check --output-last-message /workspace/run/last-message.txt - < /workspace/run/prompt.md > /workspace/run/agent.jsonl`,
  ].join('\n');

  const startedAt = Date.now();
  const result = dockerRun(
    image,
    [
      [repoDir, '/workspace/repo'],
      [agentIoDir, '/workspace/run'],
      [codexHomeDir, '/root/.codex'],
    ],
    script,
    path.join(runDir, 'agent.stderr'),
    (timeoutSeconds + 60) * 1000,
  );

  fs.writeFileSync(path.join(runDir, 'agent.stdout'), result.stdout);
  for (const file of ['agent.jsonl', 'last-message.txt']) {
    const sourcePath = path.join(agentIoDir, file);
    if (fs.existsSync(sourcePath)) {
      fs.copyFileSync(sourcePath, path.join(runDir, file));
    }
  }
  return {...result, wallTimeMs: Date.now() - startedAt};
}

function summarizeAgentJsonl(jsonlPath, referenceFiles) {
  if (!fs.existsSync(jsonlPath)) {
    return {events: 0, toolCalls: 0, failedToolCalls: 0, toolCallsByType: {}, usage: {}, firstReferenceFileEvent: null};
  }

  const lines = fs.readFileSync(jsonlPath, 'utf8').split('\n').filter(Boolean);
  const seenToolItems = new Set();
  let failedToolCalls = 0;
  const toolCallsByType = {};
  const usage = {};
  let firstReferenceFileEvent = null;

  lines.forEach((line, index) => {
    let event;
    try {
      event = JSON.parse(line);
    } catch {
      return;
    }

    const item = event.item;
    const itemType = item?.type;
    if (event.type === 'item.started' && isToolItemType(itemType)) {
      seenToolItems.add(item.id);
      toolCallsByType[itemType] = (toolCallsByType[itemType] || 0) + 1;
    } else if (event.type === 'item.completed' && isToolItemType(itemType) && !seenToolItems.has(item.id)) {
      seenToolItems.add(item.id);
      toolCallsByType[itemType] = (toolCallsByType[itemType] || 0) + 1;
    }
    if (event.type === 'item.completed' && isToolItemType(itemType) && item.status === 'failed') {
      failedToolCalls += 1;
    }

    if (firstReferenceFileEvent === null) {
      const text = JSON.stringify(event);
      const matched = referenceFiles.find(file => text.includes(file));
      if (matched) {
        firstReferenceFileEvent = {eventIndex: index, file: matched};
      }
    }

    collectUsage(event, usage);
  });

  return {
    events: lines.length,
    toolCalls: seenToolItems.size,
    failedToolCalls,
    toolCallsByType,
    usage,
    firstReferenceFileEvent,
  };
}

function isToolItemType(itemType) {
  return ['command_execution', 'file_change', 'mcp_tool_call', 'tool_call', 'function_call'].includes(itemType);
}

function collectUsage(value, usage) {
  if (!value || typeof value !== 'object') return;
  for (const [key, nested] of Object.entries(value)) {
    if (typeof nested === 'number' && /tokens?|token_count/i.test(key)) {
      usage[key] = Math.max(usage[key] || 0, nested);
    } else if (nested && typeof nested === 'object') {
      collectUsage(nested, usage);
    }
  }
}

function writeDiff(repoDir, runDir) {
  const diff = run('git', ['diff', '--binary'], {cwd: repoDir});
  const stat = run('git', ['diff', '--stat'], {cwd: repoDir});
  fs.writeFileSync(path.join(runDir, 'final.diff'), diff.stdout);
  fs.writeFileSync(path.join(runDir, 'final.diffstat'), stat.stdout);
  return {
    filesChanged: listChangedFiles(repoDir),
    diffLines: diff.stdout.split('\n').filter(Boolean).length,
  };
}

function listChangedFiles(repoDir) {
  const result = run('git', ['status', '--porcelain'], {cwd: repoDir});
  return result.stdout
    .split('\n')
    .filter(Boolean)
    .map(line => line.slice(3).trim())
    .filter(file => file !== 'IMPACT_ANALYSIS.md' && file !== 'impact-analysis.json')
    .sort();
}

function computeFileF1(predictedFiles, actualFiles) {
  const predicted = new Set(predictedFiles);
  const actual = new Set(actualFiles);
  const truePositiveFiles = predictedFiles.filter(file => actual.has(file));
  const falsePositiveFiles = predictedFiles.filter(file => !actual.has(file));
  const falseNegativeFiles = actualFiles.filter(file => !predicted.has(file));
  const precision = predictedFiles.length === 0 ? (actualFiles.length === 0 ? 1 : 0) : truePositiveFiles.length / predictedFiles.length;
  const recall = actualFiles.length === 0 ? 1 : truePositiveFiles.length / actualFiles.length;
  const f1 = precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall);
  return {precision, recall, f1, truePositiveFiles, falsePositiveFiles, falseNegativeFiles};
}

function aggregateByArm(cases) {
  const byArm = {};
  for (const arm of Array.from(new Set(cases.map(item => item.arm)))) {
    const armCases = cases.filter(item => item.arm === arm && !item.preflightOnly);
    const totals = armCases.reduce((acc, item) => {
      acc.successes += item.success ? 1 : 0;
      acc.wallTimeMs += item.wallTimeMs || 0;
      acc.toolCalls += item.agent?.toolCalls || 0;
      acc.tp += item.agentFileF1?.truePositiveFiles?.length || 0;
      acc.fp += item.agentFileF1?.falsePositiveFiles?.length || 0;
      acc.fn += item.agentFileF1?.falseNegativeFiles?.length || 0;
      for (const [key, value] of Object.entries(item.agent?.usage || {})) {
        acc.usage[key] = (acc.usage[key] || 0) + value;
      }
      return acc;
    }, {successes: 0, wallTimeMs: 0, toolCalls: 0, tp: 0, fp: 0, fn: 0, usage: {}});

    const precision = totals.tp + totals.fp === 0 ? (totals.fn === 0 ? 1 : 0) : totals.tp / (totals.tp + totals.fp);
    const recall = totals.tp + totals.fn === 0 ? 1 : totals.tp / (totals.tp + totals.fn);
    byArm[arm] = {
      cases: armCases.length,
      successRate: armCases.length === 0 ? 0 : totals.successes / armCases.length,
      averageWallTimeMs: armCases.length === 0 ? 0 : totals.wallTimeMs / armCases.length,
      averageToolCalls: armCases.length === 0 ? 0 : totals.toolCalls / armCases.length,
      fileF1: {precision, recall, f1: precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall), tp: totals.tp, fp: totals.fp, fn: totals.fn},
      usage: totals.usage,
    };
  }
  return byArm;
}

function formatList(values) {
  return values.length === 0 ? '- none' : values.map(value => `- \`${value}\``).join('\n');
}

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const manifest = JSON.parse(fs.readFileSync(args.manifest, 'utf8'));
  const cases = manifest.candidates
    .filter(candidate => !args.cases || args.cases.includes(candidate.id))
    .filter(candidate => candidate.replay);
  if (cases.length === 0) {
    const requested = args.cases ? args.cases.join(', ') : 'all cases';
    throw new Error(`No runnable replay candidates matched: ${requested}`);
  }
  const runId = new Date().toISOString().replace(/[:.]/g, '-');
  const rootOut = path.join(args.outDir, runId);
  fs.mkdirSync(rootOut, {recursive: true});

  const summary = {
    generatedAt: new Date().toISOString(),
    manifest: args.manifest,
    agentModel: args.model,
    agentRunner: 'codex-cli 0.128.0',
    image: args.image,
    preflightOnly: args.preflightOnly,
    impactContextCaveat: 'Impact arm currently uses oracle reference production files as an upper-bound file-ranking packet.',
    cases: [],
    aggregateByArm: {},
  };

  for (const candidate of cases) {
    for (const arm of args.arms) {
      const runDir = path.join(rootOut, candidate.id, arm);
      const repoDir = path.join(runDir, 'repo');
      fs.mkdirSync(runDir, {recursive: true});

      cloneCandidate(candidate, repoDir);
      applyFilesFromReference(repoDir, candidate, candidate.replay.testFiles);
      fs.writeFileSync(path.join(runDir, 'reference-production.diff'), run('git', ['diff', candidate.baseSha, candidate.mergeCommitSha, '--', ...candidate.replay.productionFiles], {cwd: repoDir}).stdout);
      fs.writeFileSync(path.join(runDir, 'regression.diff'), run('git', ['diff', '--binary'], {cwd: repoDir}).stdout);
      const verifyBroken = runRepoCommands(args.image, repoDir, candidate.replay.verifier, path.join(runDir, 'verify-broken.log'), 20 * 60 * 1000);
      if (verifyBroken.status === 0) {
        throw new Error(`Regression baseline did not fail for ${candidate.id}/${arm}`);
      }
      commitBaseline(repoDir);

      const caseSummary = {
        case: candidate.id,
        arm,
        runDir,
        repo: candidate.repo,
        pr: candidate.number,
        title: candidate.title,
        mergedAt: candidate.mergedAt,
        baseSha: candidate.baseSha,
        mergeCommitSha: candidate.mergeCommitSha,
        verifier: candidate.replay.verifier,
        referenceProductionFiles: candidate.replay.productionFiles,
        regressionFiles: candidate.replay.testFiles,
        verifyBrokenStatus: verifyBroken.status,
      };

      applyFilesFromReference(repoDir, candidate, candidate.replay.productionFiles);
      const verifyReference = runRepoCommands(args.image, repoDir, candidate.replay.verifier, path.join(runDir, 'verify-reference.log'), 20 * 60 * 1000);
      if (verifyReference.status !== 0) {
        throw new Error(`Reference production files did not pass for ${candidate.id}/${arm}\n${verifyReference.output}`);
      }
      caseSummary.verifyReferenceStatus = verifyReference.status;
      mustRun('git', ['reset', '--hard', 'HEAD'], {cwd: repoDir});
      mustRun('git', ['clean', '-fd'], {cwd: repoDir});

      if (args.preflightOnly) {
        caseSummary.preflightOnly = true;
        fs.writeFileSync(path.join(runDir, 'metrics.json'), `${JSON.stringify(caseSummary, null, 2)}\n`);
        summary.cases.push(caseSummary);
        continue;
      }

      if (arm === 'impact') {
        writeImpactFiles(runDir, repoDir, candidate);
      }
      writePrompt(runDir, candidate, arm);

      const codexHomeDir = path.join(runDir, 'codex-home');
      prepareCodexHome(args.codexHome, codexHomeDir);
      const agent = runAgent(args.image, repoDir, runDir, codexHomeDir, args.model, args.timeoutSeconds);
      const verifyAfter = runRepoCommands(args.image, repoDir, candidate.replay.verifier, path.join(runDir, 'verify-after.log'), 20 * 60 * 1000);
      const diff = writeDiff(repoDir, runDir);
      const agentSummary = summarizeAgentJsonl(path.join(runDir, 'agent.jsonl'), candidate.replay.productionFiles);
      const agentFileF1 = computeFileF1(diff.filesChanged, candidate.replay.productionFiles);
      const changedRegressionFiles = diff.filesChanged.filter(file => candidate.replay.testFiles.includes(file));

      Object.assign(caseSummary, {
        agentStatus: agent.status,
        verifyAfterStatus: verifyAfter.status,
        success: verifyAfter.status === 0 && changedRegressionFiles.length === 0,
        changedRegressionFiles,
        wallTimeMs: agent.wallTimeMs,
        agent: agentSummary,
        agentChangedFiles: diff.filesChanged,
        agentFileF1,
        diff,
      });

      fs.writeFileSync(path.join(runDir, 'metrics.json'), `${JSON.stringify(caseSummary, null, 2)}\n`);
      summary.cases.push(caseSummary);
    }
  }

  summary.aggregateByArm = aggregateByArm(summary.cases);
  fs.writeFileSync(path.join(rootOut, 'summary.json'), `${JSON.stringify(summary, null, 2)}\n`);
  console.log(`Wrote ${path.relative(repoRoot, path.join(rootOut, 'summary.json'))}`);
}

main();
