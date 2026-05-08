#!/usr/bin/env node
import {spawnSync} from 'node:child_process';
import fs from 'node:fs';
import {createRequire} from 'node:module';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import {fileURLToPath} from 'node:url';

const repoRoot = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const benchmarkRoot = path.join(repoRoot, 'benchmark', 'agent-impact');
const defaultManifest = path.join(benchmarkRoot, 'agent-impact-repos.json');
const defaultOutDir = path.join(repoRoot, 'target', 'agent-impact');
const require = createRequire(import.meta.url);
const {
  aggregateByArm,
  computeFileF1,
  isBenchmarkRelevantFile,
  summarizeAgentJsonl,
} = require('./agent-impact-metrics.cjs');

function parseArgs(argv) {
  const args = {
    manifest: defaultManifest,
    outDir: defaultOutDir,
    image: 'supermodel-agent-impact:local',
    model: null,
    codexHome: path.join(os.homedir(), '.codex'),
    cases: null,
    arms: ['control', 'impact'],
    dryRun: false,
    preflightOnly: false,
    skipBuild: false,
    timeoutSeconds: null,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const readValue = flag => {
      const value = argv[++i];
      if (value == null || value.startsWith('--')) {
        throw new Error(`Missing value for ${flag}`);
      }
      return value;
    };

    const arg = argv[i];
    if (arg === '--manifest') {
      args.manifest = path.resolve(readValue(arg));
    } else if (arg === '--out-dir') {
      args.outDir = path.resolve(readValue(arg));
    } else if (arg === '--image') {
      args.image = readValue(arg);
    } else if (arg === '--model') {
      args.model = readValue(arg);
    } else if (arg === '--codex-home') {
      args.codexHome = path.resolve(readValue(arg).replace(/^~(?=$|\/)/, os.homedir()));
    } else if (arg === '--cases') {
      args.cases = readValue(arg).split(',').map(value => value.trim()).filter(Boolean);
    } else if (arg === '--arms') {
      args.arms = readValue(arg).split(',').map(value => value.trim()).filter(Boolean);
    } else if (arg === '--timeout-seconds') {
      args.timeoutSeconds = Number(readValue(arg));
      if (!Number.isInteger(args.timeoutSeconds) || args.timeoutSeconds <= 0) {
        throw new Error('--timeout-seconds must be a positive integer');
      }
    } else if (arg === '--dry-run') {
      args.dryRun = true;
    } else if (arg === '--preflight-only') {
      args.preflightOnly = true;
    } else if (arg === '--skip-build') {
      args.skipBuild = true;
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
  console.log(`Usage: node benchmark/agent-impact/run-agent-impact-ab.mjs [options]

Options:
  --manifest <path>          Manifest path (default: benchmark/agent-impact/agent-impact-repos.json)
  --out-dir <path>           Output directory (default: target/agent-impact)
  --image <name>             Docker image with codex installed (default: supermodel-agent-impact:local)
  --model <name>             Agent model (default: manifest defaultModel)
  --codex-home <path>        Codex auth/config directory copied per run (default: ~/.codex)
  --cases <a,b,c>            Comma-separated case names
  --arms <control,impact>    Comma-separated arms to run
  --timeout-seconds <n>      Agent timeout override
  --dry-run                  Write prompts and run plan without Docker/model calls
  --preflight-only           Run install/clean verify/mutated verify, but do not invoke the agent
  --skip-build               Do not build the Docker image first
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

function buildImage(image) {
  return mustRun('docker', ['build', '-t', image, benchmarkRoot], {timeoutMs: 10 * 60 * 1000});
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

function dockerRun(image, mounts, shellScript, logPath, timeoutMs) {
  const args = ['run', '--rm'];
  for (const [host, container] of mounts) {
    args.push('-v', `${host}:${container}`);
  }
  args.push(image, 'bash', '-lc', shellScript);
  const result = run('docker', args, {timeoutMs});
  if (logPath) {
    fs.writeFileSync(logPath, result.output);
  }
  return result;
}

function cloneCase(testCase, repoDir) {
  fs.rmSync(repoDir, {recursive: true, force: true});
  fs.mkdirSync(path.dirname(repoDir), {recursive: true});
  mustRun('git', ['clone', '--no-tags', testCase.repo, repoDir], {timeoutMs: 10 * 60 * 1000});
  mustRun('git', ['checkout', '--detach', testCase.commit], {cwd: repoDir});
  mustRun('git', ['config', 'user.email', 'agent-impact-benchmark@example.invalid'], {cwd: repoDir});
  mustRun('git', ['config', 'user.name', 'Agent Impact Benchmark'], {cwd: repoDir});
  fs.appendFileSync(path.join(repoDir, '.git', 'info', 'exclude'), '\nnode_modules/\n.pnpm-store/\ndist/\ncoverage/\n');
}

function runRepoCommands(image, repoDir, commands, logPath, timeoutMs) {
  const script = [
    'set -euo pipefail',
    'cd /workspace/repo',
    ...commands,
  ].join('\n');
  return dockerRun(image, [[repoDir, '/workspace/repo']], script, logPath, timeoutMs);
}

function applyMutation(repoDir, mutation) {
  const filePath = path.join(repoDir, mutation.file);
  const before = fs.readFileSync(filePath, 'utf8');
  if (!before.includes(mutation.search)) {
    throw new Error(`Mutation search text not found in ${mutation.file}`);
  }
  fs.writeFileSync(filePath, before.replace(mutation.search, mutation.replace));
}

function mutationRetained(repoDir, mutation) {
  if (!mutation.mustContain) return true;
  const filePath = path.join(repoDir, mutation.file);
  return fs.readFileSync(filePath, 'utf8').includes(mutation.mustContain);
}

function commitMutationBaseline(repoDir, runDir, mutation) {
  const diff = run('git', ['diff', '--binary'], {cwd: repoDir});
  fs.writeFileSync(path.join(runDir, 'mutation.diff'), diff.stdout);
  mustRun('git', ['add', mutation.file], {cwd: repoDir});
  mustRun('git', ['commit', '--no-verify', '-m', 'benchmark mutation baseline'], {cwd: repoDir});
}

function buildImpactContext(repoDir, testCase) {
  const symbol = testCase.target.symbol;
  const files = [];
  walk(repoDir, file => {
    if (!/\.(ts|tsx|js|mjs|cjs|d\.ts)$/.test(file)) return;
    const relative = toPosix(path.relative(repoDir, file));
    if (relative.includes('node_modules/') || relative.includes('dist/')) return;
    const text = fs.readFileSync(file, 'utf8');
    if (text.includes(symbol.split('.').at(-1))) {
      files.push(relative);
    }
  });

  const uniqueFiles = Array.from(new Set([testCase.target.file, ...files])).sort();
  return {
    generatedBy: 'agent-impact-local-static-context',
    note: 'This is pre-mutation static context for the A/B arm. It must not include compiler ground truth.',
    target: testCase.target,
    confidenceTiers: {
      likelyAffectedFiles: uniqueFiles.filter(file => file === testCase.target.file),
      possibleContextFiles: uniqueFiles.filter(file => file !== testCase.target.file),
      architecturalContextFiles: [],
    },
  };
}

function writeImpactFiles(runDir, context) {
  fs.writeFileSync(path.join(runDir, 'impact-analysis.json'), `${JSON.stringify(context, null, 2)}\n`);
  const markdown = `# Impact Analysis

Target: \`${context.target.file}:${context.target.symbol}\`

## Likely affected files

${formatList(context.confidenceTiers.likelyAffectedFiles)}

## Possible context files

${formatList(context.confidenceTiers.possibleContextFiles)}

## Notes

${context.note}
`;
  fs.writeFileSync(path.join(runDir, 'IMPACT_ANALYSIS.md'), markdown);
}

function formatList(values) {
  return values.length === 0 ? '- none' : values.map(value => `- \`${value}\``).join('\n');
}

function writePrompt(runDir, testCase, arm) {
  const hasImpact = arm === 'impact';
  const prompt = `You are repairing a real repository benchmark case.

Repository case: ${testCase.name}
Target mutation: ${testCase.target.file}:${testCase.target.symbol}

The repository has already been mutated. Your job is to make the verifier pass while preserving the intended API/signature mutation.

Rules:
- Do not revert the target mutation.
- Do not remove the verifier or weaken tests.
- Make the smallest production-quality repair that satisfies the verifier.
- You may inspect files and run commands.
- The verifier command(s) are:
${testCase.verify.map(command => `  - ${command}`).join('\n')}

${hasImpact ? 'Impact-analysis context is available in IMPACT_ANALYSIS.md and impact-analysis.json. Use it as a starting map, but verify with the code and test output.' : 'No impact-analysis context is available for this run. Find the affected files yourself.'}

When finished, leave the repository in a passing state.`;

  fs.writeFileSync(path.join(runDir, 'prompt.md'), prompt);
}

function runAgent(image, repoDir, runDir, codexHomeDir, model, timeoutSeconds) {
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
      [runDir, '/workspace/run'],
      [codexHomeDir, '/root/.codex'],
    ],
    script,
    path.join(runDir, 'agent.stderr'),
    (timeoutSeconds + 60) * 1000,
  );

  fs.writeFileSync(path.join(runDir, 'agent.stdout'), result.stdout);
  return {...result, wallTimeMs: Date.now() - startedAt};
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
    .filter(isBenchmarkRelevantFile)
    .filter(Boolean)
    .sort();
}

function extractVerifierFiles(output, repoDir) {
  const normalizedOutput = output.replaceAll(path.sep, '/');
  const normalizedRepo = toPosix(repoDir);
  const files = listRepoFiles(repoDir)
    .filter(file => /\.(ts|tsx|js|mjs|cjs|d\.ts)$/.test(file))
    .filter(file => outputMentionsFile(normalizedOutput, file, normalizedRepo));
  return Array.from(new Set(files)).sort();
}

function outputMentionsFile(output, file, normalizedRepo) {
  const candidates = [
    file,
    `./${file}`,
    `/workspace/repo/${file}`,
    `${normalizedRepo}/${file}`,
  ];
  return candidates.some(candidate => {
    const pattern = new RegExp(`(^|[^A-Za-z0-9_./-])${escapeRegExp(candidate)}(?=$|[^A-Za-z0-9_./-])`);
    return pattern.test(output);
  });
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function listRepoFiles(repoDir) {
  const result = run('git', ['ls-files'], {cwd: repoDir});
  return result.stdout.split('\n').filter(Boolean).map(toPosix).sort();
}

function walk(dir, onFile) {
  for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
    const absolute = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!['.git', 'node_modules', 'dist', 'coverage'].includes(entry.name)) {
        walk(absolute, onFile);
      }
    } else if (entry.isFile()) {
      onFile(absolute);
    }
  }
}

function toPosix(value) {
  return value.replaceAll(path.sep, '/');
}

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const manifest = JSON.parse(fs.readFileSync(args.manifest, 'utf8'));
  const model = args.model || manifest.defaultModel || 'gpt-5.5';
  const timeoutSeconds = args.timeoutSeconds || manifest.defaultTimeoutSeconds || 1800;
  const cases = manifest.cases.filter(testCase => !args.cases || args.cases.includes(testCase.name));
  const runId = new Date().toISOString().replace(/[:.]/g, '-');
  const rootOut = path.join(args.outDir, runId);

  fs.mkdirSync(rootOut, {recursive: true});

  if (!args.dryRun && !args.skipBuild) {
    buildImage(args.image);
  }

  const summary = {
    generatedAt: new Date().toISOString(),
    manifest: args.manifest,
    agentModel: model,
    agentRunner: 'codex-cli 0.128.0',
    agentF1Definition: 'File-level F1 comparing files changed by the agent after the mutation baseline against files implicated by the broken verifier output.',
    image: args.image,
    dryRun: args.dryRun,
    preflightOnly: args.preflightOnly,
    arms: args.arms,
    cases: [],
    aggregateByArm: {},
  };

  for (const testCase of cases) {
    for (const arm of args.arms) {
      const runDir = path.join(rootOut, testCase.name, arm);
      const repoDir = path.join(runDir, 'repo');
      fs.mkdirSync(runDir, {recursive: true});

      cloneCase(testCase, repoDir);
      const impactContext = buildImpactContext(repoDir, testCase);

      if (arm === 'impact') {
        writeImpactFiles(runDir, impactContext);
        fs.copyFileSync(path.join(runDir, 'IMPACT_ANALYSIS.md'), path.join(repoDir, 'IMPACT_ANALYSIS.md'));
        fs.copyFileSync(path.join(runDir, 'impact-analysis.json'), path.join(repoDir, 'impact-analysis.json'));
      }

      writePrompt(runDir, testCase, arm);

      const caseSummary = {
        case: testCase.name,
        arm,
        runDir,
        repo: testCase.repo,
        commit: testCase.commit,
        target: testCase.target,
        dryRun: args.dryRun,
      };

      if (!args.dryRun) {
        const install = runRepoCommands(args.image, repoDir, testCase.install, path.join(runDir, 'install.log'), 20 * 60 * 1000);
        const verifyBefore = runRepoCommands(args.image, repoDir, testCase.verify, path.join(runDir, 'verify-before.log'), 20 * 60 * 1000);
        if (install.status !== 0 || verifyBefore.status !== 0) {
          throw new Error(`Clean setup failed for ${testCase.name}/${arm}`);
        }

        applyMutation(repoDir, testCase.mutation);
        const verifyBroken = runRepoCommands(args.image, repoDir, testCase.verify, path.join(runDir, 'verify-broken.log'), 20 * 60 * 1000);
        if (verifyBroken.status === 0) {
          throw new Error(`Mutation did not break verifier for ${testCase.name}/${arm}`);
        }
        const verifierImpactedFiles = extractVerifierFiles(verifyBroken.output, repoDir);
        commitMutationBaseline(repoDir, runDir, testCase.mutation);

        if (args.preflightOnly) {
          Object.assign(caseSummary, {
            agentModel: model,
            agentRunner: 'codex-cli 0.128.0',
            installStatus: install.status,
            verifyBeforeStatus: verifyBefore.status,
            verifyBrokenStatus: verifyBroken.status,
            verifierImpactedFiles,
            mutationRetained: mutationRetained(repoDir, testCase.mutation),
            preflightOnly: true,
          });
          fs.writeFileSync(path.join(runDir, 'metrics.json'), `${JSON.stringify(caseSummary, null, 2)}\n`);
          summary.cases.push(caseSummary);
          continue;
        }

        const codexHomeDir = path.join(runDir, 'codex-home');
        prepareCodexHome(args.codexHome, codexHomeDir);
        const agent = runAgent(args.image, repoDir, runDir, codexHomeDir, model, timeoutSeconds);
        const verifyAfter = runRepoCommands(args.image, repoDir, testCase.verify, path.join(runDir, 'verify-after.log'), 20 * 60 * 1000);
        const diff = writeDiff(repoDir, runDir);
        const agentSummary = summarizeAgentJsonl(path.join(runDir, 'agent.jsonl'));
        const retained = mutationRetained(repoDir, testCase.mutation);
        const agentChangedFiles = diff.filesChanged;
        const agentFileF1 = computeFileF1(agentChangedFiles, verifierImpactedFiles);

        Object.assign(caseSummary, {
          agentModel: model,
          agentRunner: 'codex-cli 0.128.0',
          installStatus: install.status,
          verifyBeforeStatus: verifyBefore.status,
          verifyBrokenStatus: verifyBroken.status,
          agentStatus: agent.status,
          verifyAfterStatus: verifyAfter.status,
          success: verifyAfter.status === 0 && retained,
          mutationRetained: retained,
          wallTimeMs: agent.wallTimeMs,
          agent: agentSummary,
          verifierImpactedFiles,
          agentChangedFiles,
          agentFileF1,
          diff,
        });

        fs.writeFileSync(path.join(runDir, 'metrics.json'), `${JSON.stringify(caseSummary, null, 2)}\n`);
      }

      summary.cases.push(caseSummary);
    }
  }

  summary.aggregateByArm = aggregateByArm(summary.cases);
  fs.writeFileSync(path.join(rootOut, 'summary.json'), `${JSON.stringify(summary, null, 2)}\n`);
  console.log(`Wrote ${path.relative(repoRoot, path.join(rootOut, 'summary.json'))}`);
}

main();
