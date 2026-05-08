const fs = require('node:fs');

function summarizeAgentJsonl(jsonlPath) {
  if (!fs.existsSync(jsonlPath)) {
    return {events: 0, toolCalls: 0, failedToolCalls: 0, toolCallsByType: {}, usage: {}};
  }

  const lines = fs.readFileSync(jsonlPath, 'utf8').split('\n').filter(Boolean);
  const seenToolItems = new Set();
  let failedToolCalls = 0;
  const toolCallsByType = {};
  const usage = {};

  for (const line of lines) {
    let event;
    try {
      event = JSON.parse(line);
    } catch {
      continue;
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

    collectUsage(event, usage);
  }

  return {
    events: lines.length,
    toolCalls: seenToolItems.size,
    failedToolCalls,
    toolCallsByType,
    usage,
  };
}

function isToolItemType(itemType) {
  return [
    'command_execution',
    'file_change',
    'mcp_tool_call',
    'tool_call',
    'function_call',
  ].includes(itemType);
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

function computeFileF1(predictedFiles, actualFiles) {
  const predicted = new Set(predictedFiles);
  const actual = new Set(actualFiles);
  const truePositiveFiles = predictedFiles.filter(file => actual.has(file));
  const falsePositiveFiles = predictedFiles.filter(file => !actual.has(file));
  const falseNegativeFiles = actualFiles.filter(file => !predicted.has(file));
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

function aggregateByArm(cases) {
  const byArm = {};
  for (const arm of Array.from(new Set(cases.map(item => item.arm)))) {
    const armCases = cases.filter(item => item.arm === arm && !item.dryRun);
    const totals = armCases.reduce((acc, item) => {
      acc.successes += item.success ? 1 : 0;
      acc.wallTimeMs += item.wallTimeMs || 0;
      acc.toolCalls += item.agent?.toolCalls || 0;
      acc.failedToolCalls += item.agent?.failedToolCalls || 0;
      acc.tp += item.agentFileF1?.truePositiveFiles?.length || 0;
      acc.fp += item.agentFileF1?.falsePositiveFiles?.length || 0;
      acc.fn += item.agentFileF1?.falseNegativeFiles?.length || 0;
      for (const [key, value] of Object.entries(item.agent?.toolCallsByType || {})) {
        acc.toolCallsByType[key] = (acc.toolCallsByType[key] || 0) + value;
      }
      for (const [key, value] of Object.entries(item.agent?.usage || {})) {
        acc.usage[key] = (acc.usage[key] || 0) + value;
      }
      return acc;
    }, {successes: 0, wallTimeMs: 0, toolCalls: 0, failedToolCalls: 0, tp: 0, fp: 0, fn: 0, toolCallsByType: {}, usage: {}});

    const precision = totals.tp + totals.fp === 0 ? (totals.fn === 0 ? 1 : 0) : totals.tp / (totals.tp + totals.fp);
    const recall = totals.tp + totals.fn === 0 ? 1 : totals.tp / (totals.tp + totals.fn);
    byArm[arm] = {
      cases: armCases.length,
      successRate: armCases.length === 0 ? 0 : totals.successes / armCases.length,
      averageWallTimeMs: armCases.length === 0 ? 0 : totals.wallTimeMs / armCases.length,
      averageToolCalls: armCases.length === 0 ? 0 : totals.toolCalls / armCases.length,
      failedToolCalls: totals.failedToolCalls,
      toolCallsByType: totals.toolCallsByType,
      fileF1: {
        precision,
        recall,
        f1: precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall),
        tp: totals.tp,
        fp: totals.fp,
        fn: totals.fn,
      },
      usage: totals.usage,
    };
  }
  return byArm;
}

function isBenchmarkRelevantFile(file) {
  const normalized = file.replaceAll('\\', '/');
  return !normalized.startsWith('node_modules/') &&
    !normalized.startsWith('.pnpm-store/') &&
    !normalized.startsWith('dist/') &&
    !normalized.startsWith('coverage/') &&
    normalized !== 'IMPACT_ANALYSIS.md' &&
    normalized !== 'impact-analysis.json' &&
    /\.(ts|tsx|js|mjs|cjs|d\.ts)$/.test(normalized);
}

module.exports = {
  aggregateByArm,
  computeFileF1,
  isBenchmarkRelevantFile,
  summarizeAgentJsonl,
};
