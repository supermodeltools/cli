# Supermodel CLI

Save 40%+ on agent token costs with code graphs.

Supermodel maps every file, function, and call relationship in your repo and writes `.graph.*` sidecar files next to your source files. Your agent reads them automatically via grep and cat. No prompt changes. No extra context windows. No new tools to learn.

📖 **Full CLI docs:** [docs.supermodeltools.com/cli/quickstart](https://docs.supermodeltools.com/cli/quickstart)

## Linux / Mac

```bash
curl -fsSL https://supermodeltools.com/install | sh
```

## npm (cross-platform)

```bash
npm install -g @supermodeltools/cli
```
---

## How it works

**1. Map your codebase**
```bash
supermodel
```
Uploads your repo to the Supermodel API, builds a full call graph, and writes `.graph.*` shard files next to every source file. Stays running to keep files updated as you code.

**2. Your agent reads the graph automatically**

`.graph.*` files are plain text. Any agent that can read files — Claude Code, Cursor, Copilot, Windsurf — picks them up automatically through its native search & file reading tools. No configuration needed on the agent side.

**3. Ask anything**

Your agent now has full visibility into your call graph, imports, domains, and blast radius — for every file in the repo, not just the ones open in the editor.

---

## Works with any AI agent

`.graph.*` files are plain text read via grep and cat. There is no agent-specific integration required.

| Agent | Setup |
|---|---|
| **Claude Code** | Run `supermodel`; install the hook for live updates (setup wizard handles this) |
| **Cursor** | Run `supermodel`; `.graph.*` files appear in context when you open any source file |
| **GitHub Copilot** | Run `supermodel`; open `.graph.*` files in the editor to include them in context |
| **Windsurf** | Same as Cursor |
| **Aider** | Run `supermodel`, then pass `--read '**/*.graph.*'` to include all graph files |
| **Any other agent** | Run `supermodel` — if it can read files, it can read `.graph.*` files |

For live updates in Claude Code, add this hook to `.claude/settings.json`:

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

The `supermodel setup` wizard installs this automatically if Claude Code is detected.

---

## Installation

### Linux / macOS (curl)

```bash
curl -fsSL https://supermodeltools.com/install | sh
```

On first run, `supermodel` launches the setup wizard automatically when attached to a terminal.

### From source

```bash
git clone https://github.com/supermodeltools/cli
cd cli
go build -o supermodel .
```

---

## Quick start

```bash
cd your/repo
supermodel                # first run: launches setup wizard, then watches for changes
```

---

## Commands

### File mode

Manages `.graph.*` shards written next to each source file. Agents read these without making API calls.

| Command | Description |
|---|---|
| `analyze [path]` | Upload repo, run full analysis, write `.graph.*` files (`--no-shards` skips file writing) |
| `skill` | Print agent awareness prompt — pipe to `CLAUDE.md` or `AGENTS.md` |
| `supermodel` | Generate graph files on startup, then keep them updated incrementally |
| `clean [path]` | Remove all `.graph.*` files from the repository |
| `hook` | Claude Code `PostToolUse` hook — forward file-change events to the `supermodel` daemon |

### Tell your agent about graph files

Add the Supermodel graph-file prompt to `CLAUDE.md`, `AGENTS.md`, or your agent's instruction file:

```bash
supermodel skill >> CLAUDE.md
```

Or manually add:

```
This repository has .graph.* files next to source files containing code relationship data from Supermodel.
For src/Foo.py, the graph file is src/Foo.graph.py.
Each .graph.* file can include [deps], [calls], and [impact] sections.
Read the .graph.* file before the source file to understand dependencies, call relationships, and blast radius before making changes.
```

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
| `compact [path]` | Strip comments and shorten identifiers to reduce token usage (aliases: `pack`, `minify`) |
| `docs [path]` | Generate a static HTML architecture documentation site |
| `restore` | Build a project context summary to restore Claude's understanding after context compaction |

### Agent integration

| Command | Description |
|---|---|
| `mcp` | Start a stdio MCP server exposing graph tools to Claude Code and other MCP hosts |

### Auth and config

| Command | Description |
|---|---|
| `setup` | Interactive setup wizard — authenticate, configure file mode, install Claude Code hook |
| `login` | Authenticate with your Supermodel account (browser or `--token` for CI) |
| `logout` | Remove stored credentials |
| `status` | Show authentication and cache status |

---

## Add a badge to your README

```markdown
[![Supermodel](https://img.shields.io/badge/supermodel-enabled-blueviolet)](https://supermodeltools.com)
```

[![Supermodel](https://img.shields.io/badge/supermodel-enabled-blueviolet)](https://supermodeltools.com)

---

## Configuration

**Save 40%+ on agent token costs — start free → [supermodeltools.com/trial](https://supermodeltools.com/trial)**

Settings are stored at `~/.supermodel/config.yaml`. Environment variables override file values.

```yaml
api_key: smsk_...        # or SUPERMODEL_API_KEY
api_base: https://...    # or SUPERMODEL_API_BASE (default: https://api.supermodeltools.com)
output: human            # human | json
shards: true             # set false to disable .graph.* writing globally (or SUPERMODEL_SHARDS=false)
```

For CI or non-interactive environments:

```bash
SUPERMODEL_API_KEY=smsk_live_... supermodel analyze
```

---

## MCP setup

To expose Supermodel graph tools directly to Claude Code via the Model Context Protocol, add the server to `~/.claude/config.json`:

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

## Links

| | |
|---|---|
| **Save 40%+ on tokens — start free** | [supermodeltools.com/trial](https://supermodeltools.com/trial) |
| **Website** | [supermodeltools.com](https://supermodeltools.com) |
| **CLI Docs** | [docs.supermodeltools.com/cli/quickstart](https://docs.supermodeltools.com/cli/quickstart) |
| **API Docs** | [api.supermodeltools.com](https://api.supermodeltools.com) |
| **Dashboard** | [dashboard.supermodeltools.com](https://dashboard.supermodeltools.com) |
| **Twitter / X** | [@supermodeltools](https://x.com/supermodeltools) |
| **Contact** | [abe@supermodel.software](mailto:abe@supermodel.software) |

---

*Questions? Open an issue or email [abe@supermodel.software](mailto:abe@supermodel.software).*
