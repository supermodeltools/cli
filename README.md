# Supermodel CLI

[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)](#installation)
[![Website](https://img.shields.io/badge/web-supermodeltools.com-blueviolet)](http://supermodeltools.com)
[![API](https://img.shields.io/badge/API-api.supermodeltools.com-blue)](https://api.supermodeltools.com)

Give your AI coding agent a map of your codebase.

Supermodel CLI connects [Claude Code](https://claude.ai/code), [Codex](https://openai.com/codex), and other AI agents to the [Supermodel API](https://api.supermodeltools.com) — providing call graphs, dependency graphs, dead code detection, and blast radius analysis as live context during your sessions.

---

## Links

| | |
|---|---|
| **Website** | [supermodeltools.com](http://supermodeltools.com) |
| **API Docs** | [api.supermodeltools.com](https://api.supermodeltools.com) |
| **Dashboard** | <!-- placeholder: dashboard URL --> |
| **Twitter / X** | <!-- placeholder: @supermodeltools --> |
| **Discord** | <!-- placeholder: invite link --> |
| **Contact** | [abe@supermodel.software](mailto:abe@supermodel.software) |

---

## What It Does

| Feature | Description |
|---|---|
| **Graph pregeneration** | Analyze your repo upfront so your agent has instant access to call and dependency graphs without waiting mid-task |
| **Dead code detection** | Surface functions and files with no callers across TypeScript, JavaScript, Python, Go, Rust, and more |
| **Blast radius** | Before making a change, show which files and functions would be affected downstream |
| **Token efficiency** | Ship only the graph slices relevant to the current task — not the whole repo — keeping context lean |
| **Agent integration** | Plug directly into Claude Code, Codex, and Hermes as a context tool |

---

## Installation

### macOS

```bash
# placeholder: brew install supermodeltools/tap/supermodel
```

### Linux

```bash
# placeholder: install script
curl -fsSL https://supermodeltools.com/install.sh | sh
```

### From Source

```bash
git clone https://github.com/supermodeltools/cli
cd cli
# placeholder: build instructions
```

---

## Quickstart

### 1. Authenticate

```bash
supermodel login
# Opens your browser for GitHub OAuth
```

### 2. Pre-generate your graph

```bash
cd /path/to/your/repo
supermodel analyze
# Uploads repo, runs analysis, caches graph locally
```

### 3. Use with your agent

**Claude Code:**
```bash
# placeholder: MCP server config or plugin setup
supermodel claude-code install
```

**Codex:**
```bash
# placeholder: Codex tool integration
supermodel codex install
```

**Hermes:**
```bash
# placeholder: Hermes integration
supermodel hermes install
```

---

## Key Commands

```bash
supermodel analyze              # Analyze the current repo and cache results
supermodel dead-code            # List functions and files with no callers
supermodel blast-radius <file>  # Show what's affected if this file changes
supermodel graph                # Print or export the graph for the current repo
supermodel status               # Show cached graph state and last analysis time
supermodel login                # Authenticate with your Supermodel account
supermodel logout               # Clear stored credentials
```

---

## How It Works

1. `supermodel analyze` zips your repository and uploads it to the [Supermodel API](https://api.supermodeltools.com).
2. The API runs static analysis — building a base IR, call graph, and domain classification.
3. Results are cached locally (and on the API) keyed by a content hash of your repo.
4. Your agent tool integration reads from the cache and injects the relevant graph slice into context.

Graph data is never sent to your AI provider directly — only the slices your agent requests.

---

## Supported Agents

| Agent | Status |
|---|---|
| Claude Code | Planned — [#1](https://github.com/supermodeltools/cli/issues/1) |
| Hermes | Planned — [#2](https://github.com/supermodeltools/cli/issues/2) |
| Codex | Planned — [#3](https://github.com/supermodeltools/cli/issues/3) |

---

## Pricing

<!-- placeholder: free plan, $19/mo Pro, $199/mo Team — link to pricing page -->

Usage is metered per analysis. Run `supermodel status` to check your balance.

---

## Contributing

Issues and PRs welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) <!-- placeholder: create this --> for guidelines.

---

*Questions? Open an issue or email [abe@supermodel.software](mailto:abe@supermodel.software).*
