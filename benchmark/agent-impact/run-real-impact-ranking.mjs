#!/usr/bin/env node
import {createRequire} from 'node:module';
import {spawnSync} from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import {fileURLToPath} from 'node:url';

const repoRoot = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const benchmarkRoot = path.join(repoRoot, 'benchmark', 'agent-impact');
const defaultManifest = path.join(benchmarkRoot, 'post-cutoff-prs.json');
const defaultOutDir = path.join(repoRoot, 'target', 'real-impact-ranking');
const publicApiRoot = path.resolve(
  process.env.SUPERMODEL_PUBLIC_API_REPO ||
  path.join(repoRoot, '..', 'supermodel-public-api')
);
const dataPlaneRoot = path.join(publicApiRoot, 'src', 'data-plane');
const dataPlaneRequire = createRequire(path.join(dataPlaneRoot, 'package.json'));

process.env.TS_NODE_TRANSPILE_ONLY = 'true';
process.env.TS_NODE_COMPILER_OPTIONS = JSON.stringify({
  module: 'commonjs',
  moduleResolution: 'node',
  experimentalDecorators: true,
  emitDecoratorMetadata: true,
});

dataPlaneRequire('ts-node/register/transpile-only');
dataPlaneRequire('reflect-metadata');

const {CodeGraphService} = dataPlaneRequire(path.join(dataPlaneRoot, 'src/services/codegraph.service.ts'));
const {HeuristicCallGraphService} = dataPlaneRequire(path.join(dataPlaneRoot, 'src/services/heuristic-call-graph.service.ts'));
const {JobWorkerService} = dataPlaneRequire(path.join(dataPlaneRoot, 'src/services/job-worker.service.ts'));
const {UUIDService} = dataPlaneRequire(path.join(dataPlaneRoot, 'src/services/uuid.service.ts'));
const {ParserOperationMode} = dataPlaneRequire(path.join(dataPlaneRoot, 'src/parsers/parser.ts'));
const {filterDependencyGraph} = dataPlaneRequire(path.join(dataPlaneRoot, 'src/services/graph-envelope.utils.ts'));

function parseArgs(argv) {
  const args = {
    manifest: defaultManifest,
    outDir: defaultOutDir,
    cases: null,
    sourceSummaryRoot: path.join(repoRoot, 'target', 'post-cutoff-pr-replay-agent'),
    maxFiles: 0,
    scope: 'replay-dirs',
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--manifest') args.manifest = path.resolve(argv[++i]);
    else if (arg === '--out-dir') args.outDir = path.resolve(argv[++i]);
    else if (arg === '--cases') args.cases = argv[++i].split(',').map(value => value.trim()).filter(Boolean);
    else if (arg === '--source-summary-root') args.sourceSummaryRoot = path.resolve(argv[++i]);
    else if (arg === '--max-files') args.maxFiles = Number(argv[++i]);
    else if (arg === '--scope') {
      args.scope = argv[++i];
      if (!['replay-dirs', 'full'].includes(args.scope)) {
        throw new Error(`Unknown scope: ${args.scope}`);
      }
    }
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
  console.log(`Usage: node benchmark/agent-impact/run-real-impact-ranking.mjs [options]

Options:
  --manifest <path>              Manifest path (default: benchmark/agent-impact/post-cutoff-prs.json)
  --out-dir <path>               Output directory (default: target/real-impact-ranking)
  --cases <a,b,c>                Comma-separated replay case ids
  --source-summary-root <path>   Existing post-cutoff replay summary root
  --max-files <n>                Optional parser file limit for smoke/debug runs only
  --scope <name>                 full or replay-dirs (default: replay-dirs)

Environment:
  SUPERMODEL_PUBLIC_API_REPO     Public API checkout that provides the data-plane implementation
`);
}

function getImpactConfig(candidate) {
  return candidate.replay || candidate.impactRanking;
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

function listSummaryFiles(root) {
  if (!fs.existsSync(root)) return [];
  const entries = fs.readdirSync(root, {withFileTypes: true})
    .filter(entry => entry.isDirectory())
    .map(entry => path.join(root, entry.name, 'summary.json'))
    .filter(file => fs.existsSync(file))
    .sort();
  return entries;
}

function findSourceRun(caseId, root) {
  const summaries = listSummaryFiles(root).reverse();
  for (const summaryPath of summaries) {
    const summary = JSON.parse(fs.readFileSync(summaryPath, 'utf8'));
    for (const item of summary.cases || []) {
      if (item.case === caseId && item.arm === 'control') {
        const repoDir = path.join(item.runDir, 'repo');
        if (fs.existsSync(path.join(repoDir, '.git'))) {
          return {summaryPath, runDir: item.runDir, repoDir};
        }
      }
    }
  }
  return null;
}

function cloneCandidate(candidate, repoDir) {
  fs.rmSync(repoDir, {recursive: true, force: true});
  fs.mkdirSync(path.dirname(repoDir), {recursive: true});
  mustRun('git', ['clone', '--filter=blob:none', '--no-tags', `https://github.com/${candidate.repo}.git`, repoDir], {
    timeoutMs: 20 * 60 * 1000,
  });
  mustRun('git', ['checkout', '--detach', candidate.baseSha], {cwd: repoDir, timeoutMs: 10 * 60 * 1000});
  mustRun('git', ['fetch', '--depth', '1', 'origin', candidate.mergeCommitSha], {cwd: repoDir, timeoutMs: 10 * 60 * 1000});
}

function prepareDirectCandidateRepo(candidate, config, repoDir, runDir) {
  cloneCandidate(candidate, repoDir);
  if (config.testFiles.length > 0) {
    mustRun('git', ['checkout', candidate.mergeCommitSha, '--', ...config.testFiles], {cwd: repoDir, timeoutMs: 10 * 60 * 1000});
  }
  const referenceDiff = run('git', ['diff', candidate.baseSha, candidate.mergeCommitSha, '--', ...config.productionFiles], {cwd: repoDir});
  if (referenceDiff.status !== 0) {
    throw new Error(`Could not create reference production diff for ${candidate.id}\n${referenceDiff.output}`);
  }
  fs.writeFileSync(path.join(runDir, 'reference-production.diff'), referenceDiff.stdout);
  return {
    summaryPath: null,
    runDir,
    repoDir,
    sourceKind: 'direct-clone',
  };
}

function prepareCleanRepo(sourceRepo, destRepo) {
  fs.rmSync(destRepo, {recursive: true, force: true});
  fs.mkdirSync(path.dirname(destRepo), {recursive: true});
  fs.cpSync(sourceRepo, destRepo, {
    recursive: true,
    filter: sourcePath => {
      const normalized = sourcePath.replaceAll(path.sep, '/');
      return !normalized.includes('/.git/') &&
        !normalized.endsWith('/.git') &&
        !normalized.includes('/node_modules/') &&
        !normalized.endsWith('/node_modules');
    },
  });

  for (const args of [
    ['ls-files', '--others', '--exclude-standard', '-z'],
    ['ls-files', '--others', '--ignored', '--exclude-standard', '-z'],
  ]) {
    const untracked = run('git', args, {cwd: sourceRepo});
    if (untracked.status === 0 && untracked.stdout) {
      for (const relPath of untracked.stdout.split('\0').filter(Boolean)) {
        fs.rmSync(path.join(destRepo, relPath), {recursive: true, force: true});
      }
    }
  }

  // Replay repos are often partial clones, so cloning or archiving HEAD can
  // force lazy blob fetches. Copy the visible working tree, then reverse only
  // the agent's local edits to recover the committed benchmark baseline.
  const diff = run('git', ['diff', '--binary'], {cwd: sourceRepo});
  if (diff.status !== 0) {
    throw new Error(`Could not read source diff from ${sourceRepo}\n${diff.output}`);
  }
  if (diff.stdout.trim()) {
    const patchPath = path.join(destRepo, '.source-working-tree.diff');
    fs.writeFileSync(patchPath, diff.stdout);
    try {
      mustRun('git', ['apply', '-R', patchPath], {cwd: destRepo});
    } finally {
      fs.rmSync(patchPath, {force: true});
    }
  }
}

function listSupportedFiles(repoDir) {
  const exts = new Set(['.ts', '.tsx', '.js', '.jsx', '.mjs', '.cjs', '.py', '.pyi', '.pyw', '.java', '.rs', '.cpp', '.cxx', '.cc', '.c', '.h', '.hpp', '.hxx', '.go', '.rb', '.swift', '.kt', '.kts', '.scala', '.hs', '.erl', '.ex', '.exs', '.html', '.json', '.sql', '.css']);
  const result = [];
  const ignored = new Set(['.git', 'node_modules', 'vendor', 'dist', 'build', 'coverage', '.next']);
  function walk(dir) {
    for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
      if (ignored.has(entry.name)) continue;
      const abs = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(abs);
      else if (entry.isFile() && exts.has(path.extname(entry.name))) result.push(path.relative(repoDir, abs).replaceAll(path.sep, '/'));
    }
  }
  walk(repoDir);
  return result.sort();
}

function isSupportedFile(file) {
  return new Set(['.ts', '.tsx', '.js', '.jsx', '.mjs', '.cjs', '.py', '.pyi', '.pyw', '.java', '.rs', '.cpp', '.cxx', '.cc', '.c', '.h', '.hpp', '.hxx', '.go', '.rb', '.swift', '.kt', '.kts', '.scala', '.hs', '.erl', '.ex', '.exs', '.html', '.json', '.sql', '.css']).has(path.extname(file));
}

function listScopedFiles(repoDir, config) {
  const dirs = unique([
    ...config.productionFiles,
    ...config.testFiles,
  ].filter(isSupportedFile).map(file => path.posix.dirname(file)));
  const all = listSupportedFiles(repoDir);
  return all.filter(file => dirs.some(dir =>
    dir === '.'
      ? !file.includes('/')
      : file === dir || file.startsWith(`${dir}/`)
  ));
}

function parseChangedOldLines(diffText, productionFiles) {
  const wanted = new Set(productionFiles);
  const changed = new Map();
  let currentFile = null;
  let oldLine = 0;
  let inHunk = false;

  for (const line of diffText.split(/\r?\n/)) {
    const fileMatch = line.match(/^\+\+\+ b\/(.+)$/);
    if (fileMatch) {
      currentFile = wanted.has(fileMatch[1]) ? fileMatch[1] : null;
      inHunk = false;
      continue;
    }
    const hunkMatch = line.match(/^@@ -(\d+)(?:,\d+)? \+\d+(?:,\d+)? @@/);
    if (hunkMatch) {
      oldLine = Number(hunkMatch[1]);
      inHunk = true;
      continue;
    }
    if (!currentFile || !inHunk) continue;
    if (!changed.has(currentFile)) changed.set(currentFile, new Set());

    if (line.startsWith('-') && !line.startsWith('---')) {
      changed.get(currentFile).add(oldLine);
      oldLine += 1;
    } else if (line.startsWith('+') && !line.startsWith('+++')) {
      changed.get(currentFile).add(Math.max(1, oldLine));
    } else {
      oldLine += 1;
    }
  }

  return changed;
}

function deriveChangedFunctionTargets(config, source, functions) {
  const diffPath = path.join(source.runDir, 'reference-production.diff');
  if (!fs.existsSync(diffPath)) return [];
  const changedByFile = parseChangedOldLines(fs.readFileSync(diffPath, 'utf8'), config.productionFiles);
  const targets = new Set();

  for (const [file, lines] of changedByFile) {
    const fileFunctions = functions
      .filter(fn => fn?.filePath === file && Number.isInteger(fn.startLine) && Number.isInteger(fn.endLine) && fn.name)
      .sort((a, b) => (a.endLine - a.startLine) - (b.endLine - b.startLine));
    for (const line of lines) {
      const match = fileFunctions.find(fn => line >= fn.startLine && line <= fn.endLine);
      if (match) targets.add(`${file}:${match.name}`);
    }
  }

  return Array.from(targets).sort();
}

function collectImpactFiles(impact) {
  const affected = [];
  const targets = [];
  const primaryValidation = [];
  const primaryValidationDetails = [];
  const validation = [];
  const validationDetails = [];
  for (const item of impact.impacts || []) {
    if (item.target?.file) targets.push(item.target.file);
    for (const entry of item.affectedFiles || []) {
      if (entry.file) affected.push(entry.file);
    }
    for (const entry of item.primaryValidationFiles || []) {
      if (entry.file) {
        primaryValidation.push(entry.file);
        primaryValidationDetails.push({
          target: item.target,
          file: entry.file,
          score: entry.score,
          confidence: entry.confidence,
          reasons: entry.reasons || [],
        });
      }
    }
    for (const entry of item.validationFiles || []) {
      if (entry.file) {
        validation.push(entry.file);
        validationDetails.push({
          target: item.target,
          file: entry.file,
          score: entry.score,
          confidence: entry.confidence,
          reasons: entry.reasons || [],
        });
      }
    }
  }
  return {
    targets: unique(targets),
    affected: unique(affected),
    primaryValidation: unique(primaryValidation),
    primaryValidationDetails,
    validation: unique(validation),
    validationDetails,
    packet: unique([...primaryValidation, ...validation, ...affected, ...targets]),
  };
}

function scoreRanked(rankedFiles, referenceFiles) {
  const ranked = unique(rankedFiles);
  const refs = unique(referenceFiles);
  const ranks = {};
  let hits = 0;
  for (const ref of refs) {
    const idx = ranked.indexOf(ref);
    ranks[ref] = idx >= 0 ? idx + 1 : null;
    if (idx >= 0) hits++;
  }
  const precision = ranked.length === 0 ? (refs.length === 0 ? 1 : 0) : hits / ranked.length;
  const recall = refs.length === 0 ? 1 : hits / refs.length;
  const f1 = precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall);
  return {
    hits,
    total: refs.length,
    precision,
    recall,
    f1,
    recallAt1: recallAt(ranked, refs, 1),
    recallAt3: recallAt(ranked, refs, 3),
    recallAt5: recallAt(ranked, refs, 5),
    ranks,
  };
}

function recallAt(ranked, refs, k) {
  if (refs.length === 0) return 1;
  const top = new Set(ranked.slice(0, k));
  return refs.filter(ref => top.has(ref)).length / refs.length;
}

function unique(values) {
  return Array.from(new Set(values.filter(Boolean)));
}

function formatNumber(value) {
  return Number.isFinite(value) ? value.toFixed(3) : String(value);
}

function writeMarkdown(report, outPath) {
  const rows = [];
  for (const result of report.results) {
    for (const mode of result.modes) {
      rows.push(`| ${result.case} | ${mode.mode} | ${mode.referenceKind} | ${formatNumber(mode.primaryValidationScore.precision)} | ${formatNumber(mode.primaryValidationScore.recall)} | ${formatNumber(mode.validationScore.recallAt1)} | ${formatNumber(mode.validationScore.recallAt3)} | ${formatNumber(mode.validationScore.recallAt5)} | ${formatNumber(mode.validationScore.precision)} | ${formatNumber(mode.packetScore.recall)} | ${formatNumber(mode.packetScore.precision)} | ${formatRanks(mode.validationScore.ranks)} |`);
    }
  }

  const details = report.results.map(result => {
    const modeDetails = result.modes.map(mode => `### ${mode.mode}

- Targets supplied to impact: ${formatList(mode.inputTargets)}
- Reference ${mode.referenceKind}: ${formatList(mode.referenceFiles)}
- Target files in response: ${formatList(mode.files.targets)}
- Affected files in response: ${formatList(mode.files.affected)}
- Primary validation files in response: ${formatList(mode.files.primaryValidation)}
- Validation files in response: ${formatList(mode.files.validation)}
- Top primary validation evidence: ${formatValidationDetails(mode.files.primaryValidationDetails)}
- Top validation evidence: ${formatValidationDetails(mode.files.validationDetails)}
- Primary validation score: recall=${formatNumber(mode.primaryValidationScore.recall)}, precision=${formatNumber(mode.primaryValidationScore.precision)}
- Validation score: recall@1=${formatNumber(mode.validationScore.recallAt1)}, recall@3=${formatNumber(mode.validationScore.recallAt3)}, recall@5=${formatNumber(mode.validationScore.recallAt5)}, precision=${formatNumber(mode.validationScore.precision)}
- Packet score: recall=${formatNumber(mode.packetScore.recall)}, precision=${formatNumber(mode.packetScore.precision)}
`).join('\n');
    return `## ${result.case}

- Repo: ${result.repo}
- Source: ${formatSource(result.source)}
- Scope: ${result.scope}
- Parsed files: ${result.graph.stats?.filesProcessed ?? 'unknown'}
- Graph nodes: ${result.graph.nodes}
- Graph relationships: ${result.graph.relationships}
- Functions: ${result.graph.functions}
- Call relationships: ${result.graph.callRelationships}

${modeDetails}`;
  }).join('\n');

  const markdown = `# Real Supermodel Impact Ranking Report

- Generated: ${report.generatedAt}
- Manifest: \`${path.relative(repoRoot, report.manifest)}\`
- Cases: ${report.results.length}
- Note: this invokes the local data-plane impact implementation. It does not use oracle PR production files except as scoring labels.

| Case | Mode | Reference | Primary Precision | Primary Recall | Validation@1 | Validation@3 | Validation@5 | Validation Precision | Packet Recall | Packet Precision | Validation Ranks |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
${rows.join('\n')}

## Interpretation

- \`production-file-targets->tests\` tests the native product shape: if this production file changes, which tests/context are structurally downstream?
- \`production-function-targets->tests\` is the sharper version: if this changed function/method changes, which tests/context are structurally downstream?
- These are scoped to the replay-relevant directories by default. The benchmark is not asking for impact of the whole repository.

${details}
`;

  fs.writeFileSync(outPath, markdown);
}

function writeAgentPacket(result, outPath) {
  const scopedModes = result.modes.filter(mode => mode.mode.startsWith('single-production-'));
  const modes = scopedModes.length > 0 ? scopedModes : result.modes;
  const sections = modes.map(mode => ({
    mode: mode.mode,
    inputTargets: mode.inputTargets,
    targetFiles: mode.files.targets,
    affectedFiles: mode.files.affected,
      validationFiles: mode.files.validationDetails.map(entry => ({
        file: entry.file,
        score: entry.score,
        confidence: entry.confidence,
        reasons: entry.reasons,
      })),
      primaryValidationFiles: mode.files.primaryValidationDetails.map(entry => ({
        file: entry.file,
        score: entry.score,
        confidence: entry.confidence,
        reasons: entry.reasons,
      })),
      packetFiles: mode.files.packet,
  }));

  const packet = {
    generatedBy: 'supermodel-real-scoped-impact-ranking',
    benchmarkCaveat: 'Generated by the local data-plane impact implementation for replay analysis. If diff-derived function targets are present, this benchmark used reference-production.diff; production use should pass user-supplied targets/diffs.',
    case: result.case,
    repo: result.repo,
    scope: result.scope,
    graph: result.graph,
    sections,
  };

  fs.writeFileSync(path.join(outPath, 'impact-analysis.scoped.json'), `${JSON.stringify(packet, null, 2)}\n`);
  fs.writeFileSync(path.join(outPath, 'IMPACT_ANALYSIS_SCOPED.md'), formatAgentPacketMarkdown(packet));
  return packet;
}

function formatAgentPacketMarkdown(packet) {
  const sections = packet.sections.map(section => `## ${section.mode}

Targets:
${formatList(section.inputTargets)}

Structurally affected files:
${formatList(section.affectedFiles)}

Validation files to inspect or run first:
${formatValidationFileList(section.validationFiles)}

Primary validation files:
${formatValidationFileList(section.primaryValidationFiles || [])}
`).join('\n');

  return `# Scoped Impact Analysis

Generated by: \`${packet.generatedBy}\`

Repository: \`${packet.repo}\`

Scope: \`${packet.scope}\`

${packet.benchmarkCaveat}

${sections}
`;
}

function formatValidationFileList(files) {
  if (!files.length) return '- none';
  return files.map(file => {
    const reasons = file.reasons?.length ? ` — ${file.reasons.join('; ')}` : '';
    return `- \`${file.file}\` (${file.confidence}, score ${file.score})${reasons}`;
  }).join('\n');
}

function formatList(values) {
  return values.length ? values.map(value => `\`${value}\``).join(', ') : '_none_';
}

function formatSource(source) {
  if (source?.summaryPath) return `replay summary \`${path.relative(repoRoot, source.summaryPath)}\``;
  if (source?.sourceKind === 'direct-clone') return 'direct clone from PR base with labeled regression files applied';
  return 'unknown';
}

function formatRanks(ranks) {
  return Object.entries(ranks).map(([file, rank]) => `${file}: ${rank ?? 'miss'}`).join('<br>');
}

function formatValidationDetails(details) {
  if (!details?.length) return '_none_';
  return details
    .slice(0, 5)
    .map(entry => `\`${entry.file}\` (${entry.score ?? 'n/a'}, ${entry.confidence ?? 'n/a'}: ${(entry.reasons || []).join('; ')})`)
    .join('; ');
}

async function analyzeCase(candidate, args, runRoot) {
  const config = getImpactConfig(candidate);
  if (!config) throw new Error(`No replay or impactRanking config found for ${candidate.id}`);

  const repoDir = path.join(runRoot, candidate.id, 'repo');
  fs.mkdirSync(path.join(runRoot, candidate.id), {recursive: true});
  let source = findSourceRun(candidate.id, args.sourceSummaryRoot);
  if (source) {
    prepareCleanRepo(source.repoDir, repoDir);
    source = {...source, sourceKind: 'replay-summary'};
  } else {
    source = prepareDirectCandidateRepo(candidate, config, repoDir, path.join(runRoot, candidate.id));
  }

  const allSupportedFiles = listSupportedFiles(repoDir);
  const scopedFiles = args.scope === 'replay-dirs' ? listScopedFiles(repoDir, config) : allSupportedFiles;
  const specificFiles = args.maxFiles > 0 ? scopedFiles.slice(0, args.maxFiles) : scopedFiles;

  process.env.REPO_ROOT = path.dirname(repoDir);
  const repoId = path.basename(repoDir);
  const userId = 'real-impact-ranking';
  const graphStart = Date.now();

  const codeGraphService = new CodeGraphService(new UUIDService());
  const worker = new JobWorkerService();
  const callGraphService = new HeuristicCallGraphService({});

  const graph = await codeGraphService.buildInMemoryGraph(repoDir, repoId, userId, ParserOperationMode.TREE_SITTER, specificFiles);
  const extracted = worker.extractFunctionsAndImports(graph);
  const convertedFunctions = worker.convertFunctionsForContext(extracted.functions);
  const callResult = await callGraphService.buildCallGraphFromContext(repoId, {
    functions: convertedFunctions,
    importsByFile: extracted.importsByFile,
  }, {resolveAmbiguous: false});
  const dependencyGraph = filterDependencyGraph(graph);

  const referenceDiffPath = path.join(source.runDir, 'reference-production.diff');
  const referenceDiff = fs.existsSync(referenceDiffPath) ? fs.readFileSync(referenceDiffPath, 'utf8') : '';
  const changedFunctionTargets = deriveChangedFunctionTargets(config, source, extracted.functions);
  const supportedTestFiles = config.testFiles.filter(file => specificFiles.includes(file));
  const modes = [
    {
      mode: 'production-file-targets->tests',
      inputTargets: config.productionFiles,
      referenceKind: 'regression test files',
      referenceFiles: supportedTestFiles,
    },
    {
      mode: 'production-function-targets->tests',
      inputTargets: changedFunctionTargets.length > 0 ? changedFunctionTargets : config.productionFiles,
      referenceKind: 'regression test files',
      referenceFiles: supportedTestFiles,
    },
  ];
  for (const target of config.productionFiles) {
    modes.push({
      mode: `single-production-file-target->tests:${target}`,
      inputTargets: [target],
      referenceKind: 'regression test files',
      referenceFiles: supportedTestFiles,
    });
  }
  for (const target of changedFunctionTargets) {
    modes.push({
      mode: `single-production-function-target->tests:${target}`,
      inputTargets: [target],
      referenceKind: 'regression test files',
      referenceFiles: supportedTestFiles,
    });
  }

  const modeResults = [];
  for (const mode of modes) {
    const impact = worker.buildImpactAnalysisResponse(
      graph,
      extracted.functions,
      extracted.classes,
      callResult,
      dependencyGraph,
      mode.inputTargets,
      new Date().toISOString(),
      new Date().toISOString(),
      null,
      {diff: referenceDiff, includeValidationFiles: true},
    );
    const files = collectImpactFiles(impact);
    modeResults.push({
      ...mode,
      files,
      affectedScore: scoreRanked(files.affected, mode.referenceFiles),
      primaryValidationScore: scoreRanked(files.primaryValidation, mode.referenceFiles),
      validationScore: scoreRanked(files.validation, mode.referenceFiles),
      packetScore: scoreRanked(files.packet, mode.referenceFiles),
      rawImpactPath: null,
    });
  }

  const packetDir = path.join(runRoot, candidate.id);
  fs.mkdirSync(packetDir, {recursive: true});
  const agentPacket = writeAgentPacket({
    case: candidate.id,
    repo: candidate.repo,
    scope: args.scope,
    graph: {
      durationMs: Date.now() - graphStart,
      nodes: graph.nodes.length,
      relationships: graph.relationships.length,
      functions: extracted.functions.length,
      callRelationships: callResult.relationships.length,
      stats: graph.stats,
    },
    modes: modeResults,
  }, packetDir);

  return {
    case: candidate.id,
    repo: candidate.repo,
    source: {
      summaryPath: source.summaryPath,
      runDir: source.runDir,
      sourceKind: source.sourceKind,
    },
    scope: args.scope,
    graph: {
      durationMs: Date.now() - graphStart,
      nodes: graph.nodes.length,
      relationships: graph.relationships.length,
      functions: extracted.functions.length,
      callRelationships: callResult.relationships.length,
      stats: graph.stats,
    },
    modes: modeResults,
    agentPacket: {
      jsonPath: path.join(packetDir, 'impact-analysis.scoped.json'),
      markdownPath: path.join(packetDir, 'IMPACT_ANALYSIS_SCOPED.md'),
      sections: agentPacket.sections.length,
    },
  };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const manifest = JSON.parse(fs.readFileSync(args.manifest, 'utf8'));
  const candidates = manifest.candidates
    .filter(candidate => getImpactConfig(candidate))
    .filter(candidate => !args.cases || args.cases.includes(candidate.id));

  const runId = new Date().toISOString().replace(/[:.]/g, '-');
  const runRoot = path.join(args.outDir, runId);
  fs.mkdirSync(runRoot, {recursive: true});

  const report = {
    generatedAt: new Date().toISOString(),
    manifest: args.manifest,
    runRoot,
    maxFiles: args.maxFiles || null,
    results: [],
  };

  for (const candidate of candidates) {
    console.error(`[real-impact] analyzing ${candidate.id}`);
    const result = await analyzeCase(candidate, args, runRoot);
    report.results.push(result);
    fs.writeFileSync(path.join(runRoot, 'summary.partial.json'), `${JSON.stringify(report, null, 2)}\n`);
  }

  fs.writeFileSync(path.join(runRoot, 'summary.json'), `${JSON.stringify(report, null, 2)}\n`);
  writeMarkdown(report, path.join(runRoot, 'report.md'));
  console.log(`Wrote ${path.join(runRoot, 'summary.json')}`);
}

main().catch(error => {
  console.error(error);
  process.exit(1);
});
