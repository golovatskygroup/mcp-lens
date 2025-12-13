# mcp-lens

## What this project is

`mcp-lens` is an MCP proxy that runs an upstream MCP server and keeps the **client tool surface small**.

Instead of exposing dozens of upstream tools at once, `mcp-lens` starts with 3 meta-tools:
- `search_tools` — search tools by keyword/category
- `describe_tool` — get a tool schema/description
- `execute_tool` — execute a discovered tool

As you use `execute_tool`, the proxy **auto-activates** underlying tools for the current session.

## How to install

### Option A (recommended): install the binary and use its absolute path

Pick your OS/arch asset from GitHub Releases and extract the `mcp-lens` binary.

Example asset names:
- `mcp-lens_1.0.2_darwin_arm64.tar.gz`
- `mcp-lens_1.0.2_linux_amd64.tar.gz`
- `mcp-lens_1.0.2_windows_amd64.zip`

#### One-liners (prints the installed absolute path)

- macOS/Linux — see the PowerShell variant below for Windows:

```bash
MCP_LENS_VERSION="${MCP_LENS_VERSION:-v1.0.2}" bash -lc 'curl -fsSL https://raw.githubusercontent.com/golovatskygroup/mcp-lens/main/install.sh | bash'
```

- Windows (PowerShell) — see the macOS/Linux variant above for bash:

```powershell
$env:MCP_LENS_VERSION = $env:MCP_LENS_VERSION; if (-not $env:MCP_LENS_VERSION) { $env:MCP_LENS_VERSION = "v1.0.2" }; iwr -useb https://raw.githubusercontent.com/golovatskygroup/mcp-lens/main/install.ps1 | iex
```

#### Client config examples (stdio)

All examples below assume the installer printed an absolute path like `/home/you/.local/bin/mcp-lens` or `C:\Users\you\AppData\Local\mcp-lens\bin\mcp-lens.exe`. Put that path into `command`.

- Cursor — see the Claude Code example below if you want CLI setup:

```json
{
  "mcpServers": {
    "tavily-proxy": {
      "command": "/ABS/PATH/TO/mcp-lens",
      "env": {
        "MCP_LENS_UPSTREAM_COMMAND": "npx",
        "MCP_LENS_UPSTREAM_ARGS_JSON": "[\"-y\",\"<TAVILY_MCP_PACKAGE>\"]",
        "MCP_LENS_UPSTREAM_ENV_JSON": "{\"TAVILY_API_KEY\":\"${TAVILY_API_KEY}\"}",
        "TAVILY_API_KEY": "YOUR_TAVILY_KEY"
      }
    }
  }
}
```

- Claude Code — see the Cursor example above if you want JSON config:

```bash
claude mcp add --transport stdio tavily-proxy --env TAVILY_API_KEY=YOUR_TAVILY_KEY -- /ABS/PATH/TO/mcp-lens
```

- Codex — use the Cursor JSON pattern above if Codex supports MCP stdio (exact support/config is currently undocumented publicly).

### Environment variables

- `MCP_LENS_PRESET` — preset name (currently: `github`)
- `MCP_LENS_UPSTREAM_COMMAND` — upstream command
- `MCP_LENS_UPSTREAM_ARGS_JSON` — args JSON array
- `MCP_LENS_UPSTREAM_ENV_JSON` — env JSON object (values support `${VAR}` expansion)

## How to contribute

- Issues/PRs are welcome.
- Do not commit real tokens/secrets.
- Local checks:
  - `go test ./...`
  - (wrapper) `npm -C packages/mcp-lens run build`

## Status

- Implemented: GitHub (preset `github`, via `@modelcontextprotocol/server-github`)
- Next: GitLab, Jira, Slack, Notion, Linear
