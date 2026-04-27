# Supermodel CLI

Save 40%+ on agent token costs with code graphs.

Supermodel maps every file, function, and call relationship in your repo and writes a `.graph` file next to each source file. Your agent reads them automatically via grep and cat. No prompt changes. No extra context windows. No new tools to learn.

## Install

```bash
npm install -g @supermodeltools/cli
```

Or via curl:

```bash
curl -fsSL https://supermodeltools.com/install.sh | sh
```

## Usage

```bash
# Generate .graph files for your repo
supermodel analyze

# Watch for changes and keep graphs up to date
supermodel watch

# Print the Claude Code skill prompt
supermodel skill

# Check for updates
supermodel update
```

## How it works

`supermodel analyze` uploads your repo to the Supermodel API, which builds a call graph and writes a small `.graph` file next to each source file. Each file contains pre-computed context: what the file exports, what it calls, what calls it, and how it connects to the rest of the codebase.

Your agent reads these files automatically when it opens or searches through your project. It spends fewer turns exploring and more turns writing.

## Benchmark

Four-way comparison on a 270k-line Django repo (8 failing tests, same model):

| | Naked Claude | + Supermodel (auto) | + Supermodel (crafted) |
|---|---|---|---|
| Cost | $0.30 | $0.15 | $0.12 |
| Turns | 20 | 11 | 9 |
| Duration | 122s | 42s | 29s |

`supermodel skill` generates the CLAUDE.md for you — no hand-tuning required.

## Links

- [supermodeltools.com](https://supermodeltools.com)
- [GitHub](https://github.com/supermodeltools/cli)
