# mcp-lens

## Title

`mcp-lens` is an MCP proxy that exposes a **single tool** to clients: `query`.

You ask in plain language → it plans a few read-only tool calls → executes them → returns structured results.

## Diagram (left → right)

![Diagram showing the mcp-lens flow](diagram.png)

## How to use it

### Install (auto, prints installed absolute path)

- macOS/Linux:

```bash
MCP_LENS_VERSION="${MCP_LENS_VERSION:-v1.0.2}" bash -lc 'curl -fsSL https://raw.githubusercontent.com/golovatskygroup/mcp-lens/main/install.sh | bash'
```

- Windows (PowerShell):

```powershell
$env:MCP_LENS_VERSION = $env:MCP_LENS_VERSION; if (-not $env:MCP_LENS_VERSION) { $env:MCP_LENS_VERSION = "v1.0.2" }; iwr -useb https://raw.githubusercontent.com/golovatskygroup/mcp-lens/main/install.ps1 | iex
```

If you cloned this repo, you can also run `./install.sh` or `./install.ps1`.


### Claude Code (1 command)

```bash
claude mcp add --transport stdio mcp-lens \
  --env OPENROUTER_API_KEY=$OPENROUTER_API_KEY \
  --env MCP_LENS_ROUTER_MODEL=$MCP_LENS_ROUTER_MODEL \
  --env JIRA_BASE_URL=$JIRA_BASE_URL \
  --env JIRA_PAT=$JIRA_PAT \
  -- /ABS/PATH/TO/mcp-lens
```

### Cursor / Codex (1 config snippet)

```json
{
  "mcpServers": {
    "mcp-lens": {
      "command": "/ABS/PATH/TO/mcp-lens",
      "env": {
        "OPENROUTER_API_KEY": "${OPENROUTER_API_KEY}",
        "MCP_LENS_ROUTER_MODEL": "${MCP_LENS_ROUTER_MODEL}",
        "GITHUB_TOKEN": "${GITHUB_TOKEN}",
        "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_PERSONAL_ACCESS_TOKEN}",
        "JIRA_BASE_URL": "${JIRA_BASE_URL}",
        "JIRA_PAT": "${JIRA_PAT}"
      }
    }
  }
}
```

### Which env vars enable which tools

- **Router / `query` tool**
  - Requires: `OPENROUTER_API_KEY` + `MCP_LENS_ROUTER_MODEL`
  - Optional tuning: `MCP_LENS_ROUTER_BASE_URL`, `MCP_LENS_ROUTER_TIMEOUT_MS`

- **GitHub local helpers (PR review / diffs / files / commits / checks)**
  - Env: `GITHUB_TOKEN` (preferred) or `GITHUB_PERSONAL_ACCESS_TOKEN` (fallback)
  - Enables router to successfully execute GitHub read-only helpers (for private repos + higher rate limits)

- **Jira local tools**
  - Env: `JIRA_BASE_URL` + one auth method:
    - DC/Server: `JIRA_PAT` (or `JIRA_BEARER_TOKEN`)
    - Cloud (basic): `JIRA_EMAIL` + `JIRA_API_TOKEN`
    - Cloud (OAuth/3LO): `JIRA_OAUTH_ACCESS_TOKEN` + `JIRA_CLOUD_ID`
  - Enables router to execute Jira read-only calls (issue search/details/comments/transitions/projects)

- **Multi-Jira routing**
  - Env: `JIRA_CLIENTS_JSON` + optional `JIRA_DEFAULT_CLIENT`
  - Lets you target a specific Jira instance by prefixing your request: `jira <client> ...`

- **Upstream MCP server selection**
  - Env: `MCP_LENS_PRESET` (e.g. `github`) or `MCP_LENS_UPSTREAM_*`
  - Changes which upstream MCP server is launched (and therefore which upstream tools exist)

## Features

- **One tool for users**: clients see only `query` via `tools/list`
- **Safer by default**: strict **read-only policy** (mutations blocked)
- **Works with big PRs**: chunked diffs + auto-pagination helpers
- **Upstream-agnostic**: runs any upstream MCP server as a child process (default: GitHub MCP)
- **Jira support included**: read-only Jira calls when auth is configured

## How it works (technically)

- **Upstream process**: launches the configured upstream MCP server (default: `@modelcontextprotocol/server-github`)
- **Registry**: loads upstream tool schemas at startup (for discovery/routing)
- **Router** (`query`):
  - sends your request + tool catalog to an LLM to produce a short JSON plan
  - validates plan against policy (read-only, allowlist, args must be JSON objects)
  - executes steps locally (helpers) or upstream (if policy allows)
  - optionally summarizes results (`include_answer=true`)
