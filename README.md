# Supermodel CLI

[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)](#installation)
[![Website](https://img.shields.io/badge/web-supermodeltools.com-blueviolet)](http://supermodeltools.com)
[![API](https://img.shields.io/badge/API-api.supermodeltools.com-blue)](https://api.supermodeltools.com)

Give your AI coding agent a map of your codebase.

Supermodel CLI connects AI coding agents to the [Supermodel API](https://api.supermodeltools.com), providing call graphs, dependency graphs, dead code detection, and blast radius analysis as context during your sessions. It operates in two modes: **file mode**, which writes `.graph.*` sidecar files next to your source so agents can read them at any time, and **on-demand analysis**, which runs targeted queries against the graph without touching the filesystem.

---

## Links

| | |
|---|---|
| **Website** | [supermodeltools.com](http://supermodeltools.com) |
| **API Docs** | [api.supermodeltools.com](https://api.supermodeltools.com) |
| **Dashboard** | [dashboard.supermodeltools.com](https://dashboard.supermodeltools.com) |
| **Twitter / X** | [@supermodeltools](https://x.com/supermodeltools) |
| **Contact** | [abe@supermodel.software](mailto:abe@supermodel.software) |

---

## Installation

### macOS

```bash
brew install supermodeltools/tap/supermodel
```

### Linux

```bash
curl -fsSL https://supermodeltools.com/install.sh | sh
```

### From source

```bash
git clone https://github.com/supermodeltools/cli
cd cli
go build -o supermodel .
```

---

## Quick start

```bash
supermodel login          # authenticate (browser or --token for CI)
cd /path/to/your/repo
supermodel analyze        # upload repo, run analysis, write .graph.* files
supermodel status         # confirm auth and cache state
```

---

## Commands

### File mode

These commands manage `.graph.*` sidecar files written next to each source file. Agents and MCP tools read these files without making API calls.

| Command | Description |
|---|---|
| `analyze [path]` | Upload repo, run full analysis, write `.graph.*` files (use `--no-files` to skip) |
| `watch [path]` | Generate graph files on startup, then keep them updated incrementally as you code |
| `clean [path]` | Remove all `.graph.*` files from the repository |
| `hook` | Claude Code `PostToolUse` hook — forward file-change events to the `watch` daemon |

### On-demand analysis

| Command | Description |
|---|---|
| `dead-code [path]` | Find unreachable functions using static analysis (aliases: `dc`) |
| `blast-radius [file]` | Show files and functions affected by changing a file, function, or diff (aliases: `br`, `impact`) |
| `audit` | Codebase health report: circular deps, coupling metrics, high blast-radius files |
| `focus <file>` | Token-efficient graph slice for a file — imports, callers, types (aliases: `ctx`, `context`) |
| `find <symbol>` | Find usages and callers of a symbol across the codebase |
| `graph [path]` | Display the full repository graph (human table, JSON, or Graphviz DOT) |

### Code tools

| Command | Description |
|---|---|
| `compact [path]` | Strip comments and shorten identifiers to reduce token usage while preserving semantics (aliases: `pack`, `minify`) |
| `docs [path]` | Generate a static HTML architecture documentation site |
| `restore` | Build a project context summary to restore Claude's understanding after a context compaction |

### Agent integration

| Command | Description |
|---|---|
| `mcp` | Start a stdio MCP server exposing graph tools to Claude Code and other MCP hosts |

### Auth and config

| Command | Description |
|---|---|
| `login` | Authenticate with your Supermodel account (browser or `--token` for CI) |
| `logout` | Remove stored credentials |
| `status` | Show authentication and cache status |

---

## Configuration

Settings are stored at `~/.supermodel/config.yaml`. Environment variables override file values.

```yaml
api_key: smsk_...        # or SUPERMODEL_API_KEY
api_base: https://...    # or SUPERMODEL_API_BASE (default: https://api.supermodeltools.com)
output: human            # human | json
files: true              # set false to disable .graph.* writing globally (or SUPERMODEL_FILES=false)
```

For CI or non-interactive environments:

```bash
SUPERMODEL_API_KEY=smsk_live_... supermodel analyze
```

---

## Claude Code integration

### Hook setup

The `hook` command forwards file-change events from Claude Code to the `supermodel watch` daemon so graph files stay current as your agent edits code. Add the following to `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "Write|Edit",
      "hooks": [{"type": "command", "command": "supermodel hook"}]
    }]
  }
}
```

Then start the daemon in your repo:

```bash
supermodel watch
```

### MCP setup

To expose Supermodel graph tools directly to Claude Code, add the MCP server to `~/.claude/config.json`:

```json
{
  "mcpServers": {
    "supermodel": {
      "command": "supermodel",
      "args": ["mcp"]
    }
  }
}
```

Exposed MCP tools: `analyze`, `dead_code`, `blast_radius`, `get_graph`.

---

*Questions? Open an issue or email [abe@supermodel.software](mailto:abe@supermodel.software).*
