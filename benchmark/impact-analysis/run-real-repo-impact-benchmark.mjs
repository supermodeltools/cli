#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const repoRoot = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const benchmarkRoot = path.join(repoRoot, 'benchmark', 'impact-analysis');
const defaultManifest = path.join(benchmarkRoot, 'real-repos.json');
const defaultOutDir = path.join(repoRoot, 'target', 'impact-analysis-real-repos');
const defaultCacheDir = path.join(defaultOutDir, 'repo-cache');
const publicApiRoot = path.resolve(
  process.env.SUPERMODEL_PUBLIC_API_REPO ||
  path.join(repoRoot, '..', 'supermodel-public-api')
);
const dataPlaneRoot = path.join(publicApiRoot, 'src', 'data-plane');

const ts = loadTypescript();

function parseArgs(argv) {
  const args = {
    manifest: defaultManifest,
    outDir: defaultOutDir,
    cacheDir: defaultCacheDir,
    keepWorkdir: false,
    algorithm: 'call-only',
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--manifest') {
      args.manifest = path.resolve(argv[++i]);
    } else if (arg === '--out-dir') {
      args.outDir = path.resolve(argv[++i]);
      if (args.cacheDir === defaultCacheDir) {
        args.cacheDir = path.join(args.outDir, 'repo-cache');
      }
    } else if (arg === '--cache-dir') {
      args.cacheDir = path.resolve(argv[++i]);
    } else if (arg === '--keep-workdir') {
      args.keepWorkdir = true;
    } else if (arg === '--algorithm') {
      args.algorithm = argv[++i];
      if (!['call-only', 'legacy-importer'].includes(args.algorithm)) {
        throw new Error(`Unknown algorithm: ${args.algorithm}`);
      }
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
  console.log(`Usage: node benchmark/impact-analysis/run-real-repo-impact-benchmark.mjs [options]

Options:
  --manifest <path>     Real repository benchmark manifest (default: benchmark/impact-analysis/real-repos.json)
  --out-dir <path>      Directory for generated reports (default: target/impact-analysis-real-repos)
  --cache-dir <path>    Directory for cloned pinned repositories (default: <out-dir>/repo-cache)
  --algorithm <name>    Prediction algorithm: call-only or legacy-importer (default: call-only)
  --keep-workdir        Keep temporary mutated workdirs for inspection
`);
}

function loadTypescript() {
  const candidates = [
    path.join(dataPlaneRoot, 'node_modules', 'typescript'),
    'typescript',
  ];
  for (const candidate of candidates) {
    try {
      return require(candidate);
    } catch {
      // Try next candidate.
    }
  }
  throw new Error('Unable to load TypeScript. Install data-plane dependencies or set up node_modules.');
}

function resolveTscBin() {
  const candidates = [
    process.env.TSC_BIN,
    path.join(dataPlaneRoot, 'node_modules', '.bin', 'tsc'),
    path.join(repoRoot, 'node_modules', '.bin', 'tsc'),
    'tsc',
  ].filter(Boolean);

  for (const candidate of candidates) {
    if (candidate === 'tsc' || fs.existsSync(candidate)) return candidate;
  }
  return 'tsc';
}

function resolveNodeTypes() {
  const candidates = [
    path.join(dataPlaneRoot, 'node_modules', '@types'),
    path.join(repoRoot, 'node_modules', '@types'),
  ];
  for (const candidate of candidates) {
    if (fs.existsSync(path.join(candidate, 'node'))) return candidate;
  }
  return '';
}

function runCommand(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    shell: false,
    timeout: 10 * 60 * 1000,
    maxBuffer: 100 * 1024 * 1024,
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

function ensurePinnedRepo(testCase, cacheDir) {
  const repoDir = path.join(cacheDir, sanitizeName(testCase.name));
  if (!fs.existsSync(path.join(repoDir, '.git'))) {
    fs.rmSync(repoDir, { recursive: true, force: true });
    fs.mkdirSync(repoDir, { recursive: true });
    mustRun('git', ['init'], repoDir);
    mustRun('git', ['remote', 'add', 'origin', testCase.repo], repoDir);
  }

  const current = runCommand('git', ['rev-parse', 'HEAD'], repoDir);
  if (current.status !== 0 || current.stdout.trim() !== testCase.commit) {
    mustRun('git', ['fetch', '--depth', '1', 'origin', testCase.commit], repoDir);
    mustRun('git', ['checkout', '--detach', testCase.commit], repoDir);
  }

  const verified = runCommand('git', ['rev-parse', 'HEAD'], repoDir);
  if (verified.status !== 0 || verified.stdout.trim() !== testCase.commit) {
    throw new Error(`Could not verify ${testCase.name} at ${testCase.commit}`);
  }

  return repoDir;
}

function mustRun(command, args, cwd) {
  const result = runCommand(command, args, cwd);
  if (result.status !== 0) {
    throw new Error(`Command failed: ${result.command}\n${result.output}`);
  }
  return result;
}

function copyRepo(repoDir, tempRoot, label) {
  const dest = path.join(tempRoot, label);
  fs.cpSync(repoDir, dest, {
    recursive: true,
    filter: (src) => {
      const normalized = src.replaceAll(path.sep, '/');
      return !normalized.includes('/.git/') &&
        !normalized.endsWith('/.git') &&
        !normalized.includes('/node_modules/') &&
        !normalized.endsWith('/node_modules') &&
        !normalized.includes('/dist/') &&
        !normalized.endsWith('/dist') &&
        !normalized.includes('/coverage/') &&
        !normalized.endsWith('/coverage');
    },
  });
  return dest;
}

function applyMutation(workdir, mutation) {
  const filePath = path.join(workdir, mutation.file);
  const before = fs.readFileSync(filePath, 'utf8');
  if (!before.includes(mutation.search)) {
    throw new Error(`Mutation search text not found in ${mutation.file}`);
  }
  fs.writeFileSync(filePath, before.replace(mutation.search, mutation.replace));
}

function runCheck(workdir, check, context) {
  const command = check.command === 'tsc' ? resolveTscBin() : check.command;
  const args = (check.args || []).flatMap((arg) => expandArgGlob(expandArg(arg, context), workdir));
  return runCommand(command, args, workdir);
}

function expandArg(arg, context) {
  return String(arg)
    .replaceAll('${nodeTypes}', context.nodeTypes)
    .replaceAll('${repoRoot}', repoRoot);
}

function expandArgGlob(arg, workdir) {
  if (!arg.includes('*')) return [arg];
  const normalized = arg.replaceAll(path.sep, '/');
  const slashIdx = normalized.lastIndexOf('/');
  const dir = slashIdx >= 0 ? normalized.slice(0, slashIdx) : '.';
  const pattern = slashIdx >= 0 ? normalized.slice(slashIdx + 1) : normalized;
  const absDir = path.join(workdir, dir);
  if (!fs.existsSync(absDir)) return [arg];

  const regex = new RegExp(`^${escapeRegExp(pattern).replaceAll('\\*', '.*')}$`);
  const matches = fs.readdirSync(absDir)
    .filter((entry) => regex.test(entry))
    .map((entry) => toPosix(path.posix.join(dir, entry)))
    .sort();
  return matches.length > 0 ? matches : [arg];
}

function buildGraph(workdir, testCase) {
  const sourceFiles = listSourceFiles(workdir, testCase.sourceRoots || ['src']);
  const allFiles = new Set(sourceFiles);
  const fileImportedBy = new Map();
  const importsByFile = new Map();
  const namespaceImportsByFile = new Map();
  const functionsByFile = new Map();
  const functionsById = new Map();
  const exportsByFile = new Map();
  const localsByFile = new Map();
  const sourceByFile = new Map();

  for (const file of sourceFiles) {
    const absPath = path.join(workdir, file);
    const sourceText = fs.readFileSync(absPath, 'utf8');
    const sourceFile = ts.createSourceFile(file, sourceText, ts.ScriptTarget.Latest, true);
    sourceByFile.set(file, sourceFile);
    importsByFile.set(file, new Map());
    namespaceImportsByFile.set(file, new Map());
    functionsByFile.set(file, []);
    exportsByFile.set(file, new Map());
    localsByFile.set(file, new Map());
  }

  for (const file of sourceFiles) {
    const sourceFile = sourceByFile.get(file);
    discoverImports(file, sourceFile, allFiles, importsByFile.get(file), namespaceImportsByFile.get(file), fileImportedBy);
    discoverFunctions(file, sourceFile, functionsByFile, functionsById, exportsByFile, localsByFile);
  }

  const callersByCallee = new Map();
  const relationships = [];
  for (const file of sourceFiles) {
    const sourceFile = sourceByFile.get(file);
    for (const fn of functionsByFile.get(file) || []) {
      for (const calleeId of findCalls(fn, file, sourceFile, importsByFile, namespaceImportsByFile, exportsByFile, localsByFile)) {
        if (!callersByCallee.has(calleeId)) callersByCallee.set(calleeId, new Set());
        callersByCallee.get(calleeId).add(fn.id);
        relationships.push({ caller: fn.id, callee: calleeId });
      }
    }
  }

  return {
    sourceFiles,
    functionsByFile,
    functionsById,
    exportsByFile,
    localsByFile,
    importsByFile,
    namespaceImportsByFile,
    fileImportedBy,
    callersByCallee,
    relationships,
  };
}

function listSourceFiles(workdir, sourceRoots) {
  const files = [];
  for (const root of sourceRoots) {
    const absRoot = path.join(workdir, root);
    if (!fs.existsSync(absRoot)) continue;
    walk(absRoot, (file) => {
      if (/\.(ts|tsx|js|mjs|cjs)$/.test(file) && !file.endsWith('.d.ts')) {
        files.push(toPosix(path.relative(workdir, file)));
      }
    });
  }
  return files.sort();
}

function walk(dir, onFile) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const abs = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!['node_modules', 'dist', 'coverage', '.git'].includes(entry.name)) {
        walk(abs, onFile);
      }
    } else if (entry.isFile()) {
      onFile(abs);
    }
  }
}

function discoverImports(file, sourceFile, allFiles, namedImports, namespaceImports, fileImportedBy) {
  for (const statement of sourceFile.statements) {
    if (ts.isImportDeclaration(statement) && ts.isStringLiteral(statement.moduleSpecifier)) {
      const targetFile = resolveRelativeImport(file, statement.moduleSpecifier.text, allFiles);
      if (!targetFile) continue;
      addFileImporter(fileImportedBy, targetFile, file);
      const importClause = statement.importClause;
      if (!importClause) continue;
      if (importClause.name) {
        namedImports.set(importClause.name.text, { file: targetFile, importedName: 'default' });
      }
      const namedBindings = importClause.namedBindings;
      if (namedBindings && ts.isNamedImports(namedBindings)) {
        for (const element of namedBindings.elements) {
          namedImports.set(element.name.text, {
            file: targetFile,
            importedName: (element.propertyName || element.name).text,
          });
        }
      } else if (namedBindings && ts.isNamespaceImport(namedBindings)) {
        namespaceImports.set(namedBindings.name.text, targetFile);
      }
    } else if (ts.isExportDeclaration(statement) && statement.moduleSpecifier && ts.isStringLiteral(statement.moduleSpecifier)) {
      const targetFile = resolveRelativeImport(file, statement.moduleSpecifier.text, allFiles);
      if (targetFile) addFileImporter(fileImportedBy, targetFile, file);
    }
  }
}

function addFileImporter(fileImportedBy, targetFile, importerFile) {
  if (!fileImportedBy.has(targetFile)) fileImportedBy.set(targetFile, new Set());
  fileImportedBy.get(targetFile).add(importerFile);
}

function discoverFunctions(file, sourceFile, functionsByFile, functionsById, exportsByFile, localsByFile) {
  const addFunction = (node, nameNode, name, exported) => {
    const id = `${file}:${name}:${node.pos}`;
    const line = sourceFile.getLineAndCharacterOfPosition(nameNode.getStart(sourceFile)).line + 1;
    const info = {
      id,
      file,
      name,
      line,
      exported,
      node,
      body: node.body,
      start: node.getStart(sourceFile),
      end: node.getEnd(),
    };
    functionsByFile.get(file).push(info);
    functionsById.set(id, info);
    localsByFile.get(file).set(name, id);
    if (exported) exportsByFile.get(file).set(name, id);
  };

  const visit = (node) => {
    if (ts.isFunctionDeclaration(node) && node.name) {
      addFunction(node, node.name, node.name.text, hasExportModifier(node));
      return;
    }

    if (ts.isVariableStatement(node)) {
      const exported = hasExportModifier(node);
      for (const declaration of node.declarationList.declarations) {
        if (
          ts.isIdentifier(declaration.name) &&
          declaration.initializer &&
          (ts.isArrowFunction(declaration.initializer) || ts.isFunctionExpression(declaration.initializer))
        ) {
          addFunction(declaration.initializer, declaration.name, declaration.name.text, exported);
        }
      }
      return;
    }

    if (ts.isClassDeclaration(node) && node.name) {
      for (const member of node.members) {
        if ((ts.isMethodDeclaration(member) || ts.isGetAccessorDeclaration(member) || ts.isSetAccessorDeclaration(member)) && member.name) {
          const name = propertyNameText(member.name);
          if (name) addFunction(member, member.name, `${node.name.text}.${name}`, hasExportModifier(node));
        }
      }
      return;
    }

    ts.forEachChild(node, visit);
  };

  ts.forEachChild(sourceFile, visit);
}

function hasExportModifier(node) {
  return Boolean(ts.getCombinedModifierFlags(node) & ts.ModifierFlags.Export);
}

function propertyNameText(name) {
  if (ts.isIdentifier(name) || ts.isStringLiteral(name) || ts.isNumericLiteral(name)) return name.text;
  if (ts.isPrivateIdentifier(name)) return name.text;
  return null;
}

function findCalls(fn, file, sourceFile, importsByFile, namespaceImportsByFile, exportsByFile, localsByFile) {
  const callees = new Set();
  if (!fn.body) return [];

  const visit = (node) => {
    if (node !== fn.body && isFunctionLikeWithBody(node)) {
      return;
    }

    if (ts.isCallExpression(node) || ts.isNewExpression(node)) {
      const resolved = resolveCallExpression(node.expression, file, importsByFile, namespaceImportsByFile, exportsByFile, localsByFile);
      if (resolved) callees.add(resolved);
    }

    ts.forEachChild(node, visit);
  };

  visit(fn.body);
  return Array.from(callees);
}

function isFunctionLikeWithBody(node) {
  return (ts.isFunctionDeclaration(node) ||
    ts.isFunctionExpression(node) ||
    ts.isArrowFunction(node) ||
    ts.isMethodDeclaration(node) ||
    ts.isGetAccessorDeclaration(node) ||
    ts.isSetAccessorDeclaration(node)) && node.body;
}

function resolveCallExpression(expression, file, importsByFile, namespaceImportsByFile, exportsByFile, localsByFile) {
  if (ts.isIdentifier(expression)) {
    const localId = localsByFile.get(file)?.get(expression.text);
    if (localId) return localId;

    const imported = importsByFile.get(file)?.get(expression.text);
    if (imported) return exportsByFile.get(imported.file)?.get(imported.importedName) || null;
  }

  if (ts.isPropertyAccessExpression(expression)) {
    if (ts.isIdentifier(expression.expression)) {
      const namespaceFile = namespaceImportsByFile.get(file)?.get(expression.expression.text);
      if (namespaceFile) {
        return exportsByFile.get(namespaceFile)?.get(expression.name.text) || null;
      }
    }
  }

  return null;
}

function resolveRelativeImport(sourceFile, specifier, allFiles) {
  if (!specifier.startsWith('.')) return null;
  const sourceDir = path.posix.dirname(sourceFile);
  const base = path.posix.normalize(path.posix.join(sourceDir, specifier));
  const withoutJs = base.replace(/\.(mjs|cjs|js|jsx)$/i, '');
  const candidates = [
    base,
    withoutJs,
    `${withoutJs}.ts`,
    `${withoutJs}.tsx`,
    `${withoutJs}.js`,
    `${withoutJs}.mjs`,
    `${withoutJs}.cjs`,
    `${withoutJs}/index.ts`,
    `${withoutJs}/index.tsx`,
    `${withoutJs}/index.js`,
    `${withoutJs}/index.mjs`,
    `${withoutJs}/index.cjs`,
  ];
  return candidates.find((candidate) => allFiles.has(candidate)) || null;
}

function predictImpact(graph, target, algorithm = 'call-only') {
  const targetId = graph.localsByFile.get(target.file)?.get(target.name) ||
    graph.exportsByFile.get(target.file)?.get(target.name);
  if (!targetId) {
    throw new Error(`Could not resolve target ${target.file}:${target.name}`);
  }

  const visited = new Map([[targetId, 0]]);
  const queue = [{ id: targetId, distance: 0 }];

  if (algorithm === 'legacy-importer') {
    const fileVisited = new Set([target.file]);
    const fileQueue = [{ file: target.file, distance: 0 }];
    while (fileQueue.length > 0) {
      const { file, distance } = fileQueue.shift();
      const importers = graph.fileImportedBy.get(file);
      if (!importers) continue;
      for (const importer of importers) {
        if (fileVisited.has(importer)) continue;
        fileVisited.add(importer);
        fileQueue.push({ file: importer, distance: distance + 1 });
        for (const fn of graph.functionsByFile.get(importer) || []) {
          if (!visited.has(fn.id)) {
            visited.set(fn.id, distance + 1);
            queue.push({ id: fn.id, distance: distance + 1 });
          }
        }
      }
    }
  }

  while (queue.length > 0) {
    const { id, distance } = queue.shift();
    const callers = graph.callersByCallee.get(id);
    if (!callers) continue;
    for (const callerId of callers) {
      if (!visited.has(callerId)) {
        visited.set(callerId, distance + 1);
        queue.push({ id: callerId, distance: distance + 1 });
      }
    }
  }

  const affectedFunctions = [];
  const fileCounts = new Map();
  for (const [nodeId, distance] of visited) {
    if (distance === 0) continue;
    const fn = graph.functionsById.get(nodeId);
    if (!fn) continue;
    affectedFunctions.push({
      file: fn.file,
      name: fn.name,
      line: fn.line,
      distance,
      relationship: distance === 1 ? 'direct_caller' : 'transitive_caller',
    });
    const counts = fileCounts.get(fn.file) || { directDependencies: 0, transitiveDependencies: 0 };
    if (distance === 1) counts.directDependencies += 1;
    else counts.transitiveDependencies += 1;
    fileCounts.set(fn.file, counts);
  }

  affectedFunctions.sort((a, b) => a.distance - b.distance || a.file.localeCompare(b.file) || a.line - b.line);

  const affectedFiles = Array.from(fileCounts, ([file, counts]) => ({ file, ...counts }))
    .sort((a, b) => {
      const aScore = a.directDependencies + a.transitiveDependencies;
      const bScore = b.directDependencies + b.transitiveDependencies;
      return bScore - aScore || a.file.localeCompare(b.file);
    });

  return {
    target,
    affectedFunctions,
    affectedFiles,
    blastRadius: {
      directDependents: affectedFunctions.filter((fn) => fn.distance === 1).length,
      transitiveDependents: affectedFunctions.filter((fn) => fn.distance > 1).length,
      affectedFiles: affectedFiles.length,
    },
  };
}

function extractFilesFromOutput(output, workdir, sourceRoots) {
  const files = new Set();
  const normalized = output.replaceAll(path.sep, '/');
  const normalizedWorkdir = workdir.replaceAll(path.sep, '/');
  const rootPattern = sourceRoots.map(escapeRegExp).join('|');
  const patterns = [
    new RegExp(`(?:^|\\n)((${rootPattern})/[^:(\\n]+\\.(?:ts|tsx|js|mjs|cjs))(?:\\(\\d+,\\d+\\)|:\\d+:\\d+)?:\\s+error\\b`, 'g'),
    new RegExp(`${escapeRegExp(normalizedWorkdir)}/((${rootPattern})/[^:(\\n]+\\.(?:ts|tsx|js|mjs|cjs))(?:\\(\\d+,\\d+\\)|:\\d+:\\d+)?:\\s+error\\b`, 'g'),
  ];
  for (const pattern of patterns) {
    let match;
    while ((match = pattern.exec(normalized)) !== null) {
      files.add(match[1]);
    }
  }
  return Array.from(files).sort();
}

function computeMetrics(predictedFiles, actualFiles) {
  const predicted = new Set(predictedFiles);
  const actual = new Set(actualFiles);
  const truePositiveFiles = predictedFiles.filter((file) => actual.has(file));
  const falsePositiveFiles = predictedFiles.filter((file) => !actual.has(file));
  const falseNegativeFiles = actualFiles.filter((file) => !predicted.has(file));
  const precision = predictedFiles.length === 0 ? (actualFiles.length === 0 ? 1 : 0) : truePositiveFiles.length / predictedFiles.length;
  const recall = actualFiles.length === 0 ? 1 : truePositiveFiles.length / actualFiles.length;
  const f1 = precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall);
  return {
    precision,
    recall,
    f1,
    truePositiveFiles,
    falsePositiveFiles,
    falseNegativeFiles,
  };
}

function summarizeCheck(check, workdir, sourceRoots) {
  return {
    command: check.command,
    status: check.status,
    signal: check.signal,
    implicatedFiles: extractFilesFromOutput(check.output, workdir, sourceRoots),
    stdout: check.stdout,
    stderr: check.stderr,
  };
}

function aggregateMetrics(cases) {
  const totals = cases.reduce((acc, testCase) => {
    acc.tp += testCase.metrics.truePositiveFiles.length;
    acc.fp += testCase.metrics.falsePositiveFiles.length;
    acc.fn += testCase.metrics.falseNegativeFiles.length;
    return acc;
  }, { tp: 0, fp: 0, fn: 0 });

  const precision = totals.tp + totals.fp === 0 ? (totals.fn === 0 ? 1 : 0) : totals.tp / (totals.tp + totals.fp);
  const recall = totals.tp + totals.fn === 0 ? 1 : totals.tp / (totals.tp + totals.fn);
  const f1 = precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall);
  const macroF1 = cases.length === 0 ? 0 : cases.reduce((sum, testCase) => sum + testCase.metrics.f1, 0) / cases.length;
  return { ...totals, precision, recall, f1, macroF1 };
}

function writeMarkdownReport(report, outPath) {
  const rows = report.cases.map((testCase) => {
    const metrics = testCase.metrics;
    return `| ${testCase.name} | ${testCase.repoName} | ${testCase.target.file}:${testCase.target.name} | ${formatNumber(metrics.precision)} | ${formatNumber(metrics.recall)} | ${formatNumber(metrics.f1)} | ${metrics.truePositiveFiles.length} | ${metrics.falsePositiveFiles.length} | ${metrics.falseNegativeFiles.length} |`;
  }).join('\n');

  const details = report.cases.map((testCase) => `### ${testCase.name}

- Repository: ${testCase.repo} @ \`${testCase.commit}\`
- Target: \`${testCase.target.file}:${testCase.target.name}\`
- Mutation: ${testCase.mutation.description}
- Check: \`${testCase.check.command}\`
- Predicted files: ${formatList(testCase.predictedFiles)}
- Actual broken files: ${formatList(testCase.actualFiles)}
- False positives: ${formatList(testCase.metrics.falsePositiveFiles)}
- False negatives: ${formatList(testCase.metrics.falseNegativeFiles)}
`).join('\n');

  const markdown = `# Real Repository Impact Analysis Benchmark

- Manifest: \`${path.relative(repoRoot, report.manifest)}\`
- Generated: ${report.generatedAt}
- Cases: ${report.cases.length}
- Micro precision: ${formatNumber(report.aggregate.precision)}
- Micro recall: ${formatNumber(report.aggregate.recall)}
- Micro F1: ${formatNumber(report.aggregate.f1)}
- Macro F1: ${formatNumber(report.aggregate.macroF1)}

| Case | Repo | Target | Precision | Recall | F1 | TP | FP | FN |
|---|---|---|---:|---:|---:|---:|---:|---:|
${rows}

## Details

${details}
`;

  fs.writeFileSync(outPath, markdown);
}

function formatNumber(value) {
  return Number.isFinite(value) ? value.toFixed(3) : String(value);
}

function formatList(values) {
  return values.length ? values.map((value) => `\`${value}\``).join(', ') : '_none_';
}

function sanitizeName(name) {
  return name.replace(/[^a-zA-Z0-9._-]/g, '-');
}

function repoName(repoUrl) {
  return repoUrl.replace(/\.git$/, '').split('/').slice(-2).join('/');
}

function toPosix(value) {
  return value.replaceAll(path.sep, '/');
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const manifest = JSON.parse(fs.readFileSync(args.manifest, 'utf8'));
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'supermodel-real-impact-'));
  fs.mkdirSync(args.outDir, { recursive: true });
  fs.mkdirSync(args.cacheDir, { recursive: true });

  const context = {
    nodeTypes: resolveNodeTypes(),
  };

  const report = {
    generatedAt: new Date().toISOString(),
    manifest: args.manifest,
    name: manifest.name,
    description: manifest.description,
    algorithm: args.algorithm,
    repoRoot,
    tscBin: resolveTscBin(),
    nodeTypes: context.nodeTypes,
    cases: [],
    aggregate: null,
    tempRoot: args.keepWorkdir ? tempRoot : undefined,
  };

  try {
    for (const testCase of manifest.cases) {
      const cachedRepo = ensurePinnedRepo(testCase, args.cacheDir);
      const baselineDir = copyRepo(cachedRepo, tempRoot, `${sanitizeName(testCase.name)}-baseline`);
      const graph = buildGraph(baselineDir, testCase);
      const prediction = predictImpact(graph, testCase.target, args.algorithm);
      const predictedFiles = prediction.affectedFiles.map((entry) => entry.file);

      const baselineCheck = runCheck(baselineDir, testCase.check, context);
      if (baselineCheck.status !== 0) {
        throw new Error(`Baseline check failed for ${testCase.name}:\n${baselineCheck.output}`);
      }

      const mutationDir = copyRepo(cachedRepo, tempRoot, `${sanitizeName(testCase.name)}-mutated`);
      applyMutation(mutationDir, testCase.mutation);
      const mutationCheck = runCheck(mutationDir, testCase.check, context);
      const actualFiles = extractFilesFromOutput(mutationCheck.output, mutationDir, testCase.sourceRoots || ['src']);
      if (actualFiles.length === 0) {
        throw new Error(`Mutation produced no compiler-implicated source files for ${testCase.name}`);
      }
      const metrics = computeMetrics(predictedFiles, actualFiles);

      report.cases.push({
        name: testCase.name,
        repo: testCase.repo,
        repoName: repoName(testCase.repo),
        commit: testCase.commit,
        target: testCase.target,
        mutation: testCase.mutation,
        check: {
          command: summarizeCheck(mutationCheck, mutationDir, testCase.sourceRoots || ['src']).command,
          args: testCase.check.args,
        },
        graph: {
          sourceFiles: graph.sourceFiles.length,
          functions: graph.functionsById.size,
          callRelationships: graph.relationships.length,
        },
        prediction,
        predictedFiles,
        actualFiles,
        baseline: summarizeCheck(baselineCheck, baselineDir, testCase.sourceRoots || ['src']),
        mutated: summarizeCheck(mutationCheck, mutationDir, testCase.sourceRoots || ['src']),
        metrics,
      });
    }

    report.aggregate = aggregateMetrics(report.cases);
  } finally {
    if (!report.aggregate) report.aggregate = aggregateMetrics(report.cases);
    const jsonPath = path.join(args.outDir, 'real-repo-impact-report.json');
    const mdPath = path.join(args.outDir, 'real-repo-impact-report.md');
    fs.writeFileSync(jsonPath, `${JSON.stringify(report, null, 2)}\n`);
    writeMarkdownReport(report, mdPath);
    console.log(`Wrote ${path.relative(repoRoot, jsonPath)}`);
    console.log(`Wrote ${path.relative(repoRoot, mdPath)}`);
    if (!args.keepWorkdir) fs.rmSync(tempRoot, { recursive: true, force: true });
  }

  if (report.aggregate.f1 < 0.8) {
    process.exitCode = 1;
  }
}

main();
