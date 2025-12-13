# mcp-lens

A tiny **MCP proxy** that wraps upstream MCP servers and exposes a *smaller*, easier tool surface to the client.

Instead of loading every upstream tool into your client, this proxy starts the upstream server and provides **3 meta-tools**:
- `search_tools` — find available tools by keyword/category
- `describe_tool` — get a tool schema/description
- `execute_tool` — execute a discovered tool

As you use `execute_tool`, the proxy **auto-activates** the underlying tools for the current session.

## Status & Roadmap

- **Implemented**: GitHub (via `@modelcontextprotocol/server-github`)
- **Next**: GitLab, Jira, Slack, Notion, Linear

## Why this is useful (vs “just GitHub tools”)

When you connect a raw GitHub MCP server directly, clients typically see *dozens* of tools at once.
That creates practical problems:

- **Tool overload**: big tool lists reduce usability and often reduce tool-selection quality.
- **Harder discovery**: users don’t remember exact tool names.
- **Heavier prompts**: large tool schemas can bloat context.

This proxy keeps the client surface minimal and makes discovery explicit.

## Requirements

- Go 1.22+ (to build)
- Node.js + `npx` available (the proxy runs the upstream GitHub MCP server via `npx`)
- A GitHub token in your environment (`GITHUB_TOKEN`)

## Install

### Option A — Build from source

```bash
go build -o mcp-lens ./cmd/proxy
```

### Option B — Download from GitHub Releases

For most users, the easiest path is to download the latest `mcp-lens` binary from GitHub Releases.
Pick the archive matching your OS/arch (e.g. `mcp-lens_1.0.2_darwin_arm64.tar.gz`, `mcp-lens_1.0.2_linux_amd64.tar.gz`, `mcp-lens_1.0.2_windows_amd64.zip`).

### Option C — Run via npx (auto-download)

If you don’t want to pre-download a binary, you can run `mcp-lens` via an npx wrapper that downloads the correct GitHub Release asset, verifies `checksums.txt`, caches it, and then executes.

```bash
npx -y @golovatskygroup/mcp-lens
```

## Configuration

### Quick Start — Environment Variables

The simplest way to run `mcp-lens` is by setting environment variables.

Default behavior is equivalent to `MCP_LENS_PRESET=github`.

```bash
export MCP_LENS_PRESET="github"
export GITHUB_TOKEN="your_github_token_here"
./mcp-lens
```

#### ENV reference

- `MCP_LENS_PRESET` 		Preset name (currently: `github`)
- `MCP_LENS_UPSTREAM_COMMAND` 	Override upstream command
- `MCP_LENS_UPSTREAM_ARGS_JSON` 	JSON array of args, e.g. `'["-y","@modelcontextprotocol/server-github"]'`
- `MCP_LENS_UPSTREAM_ENV_JSON` 	JSON object of env vars to merge, e.g. `'{"GITHUB_PERSONAL_ACCESS_TOKEN":"${GITHUB_TOKEN}"}'`

(Values support `${VAR}` expansion.)

#### Custom upstream example (no YAML)

```bash
export MCP_LENS_UPSTREAM_COMMAND="/abs/path/to/your-mcp-server"
export MCP_LENS_UPSTREAM_ARGS_JSON='["--stdio"]'
export MCP_LENS_UPSTREAM_ENV_JSON='{"API_TOKEN":"${API_TOKEN}"}'
./mcp-lens
```

#### Using npx wrapper with env

```bash
export MCP_LENS_PRESET="github"
export GITHUB_TOKEN="your_github_token_here"
npx -y @golovatskygroup/mcp-lens
```

### Advanced — YAML Config

For more control, you can pass a custom config file:

```bash
./mcp-lens -config ./config.example.yaml
```

Example `config.example.yaml`:

```yaml
upstream:
  command: "npx"
  args:
    - "-y"
    - "@modelcontextprotocol/server-github"
  env:
    GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"
```

## Add to your editor / client

Below are minimal MCP stdio examples.

### Claude Desktop

Add a server entry pointing to the `mcp-lens` binary with environment variables:

```json
{
  "mcpServers": {
    "github-proxy": {
      "command": "/ABS/PATH/TO/mcp-lens",
      "env": {
        "GITHUB_TOKEN": "YOUR_TOKEN"
      }
    }
  }
}
```

#### Option C — Using npx wrapper (auto-download mcp-lens)

This runs `mcp-lens` via `npx` (the wrapper downloads the matching GitHub Release, verifies checksums, caches, then execs).

```json
{
  "mcpServers": {
    "github-proxy": {
      "command": "npx",
      "args": ["-y", "@golovatskygroup/mcp-lens"],
      "env": {
        "MCP_LENS_PRESET": "github",
        "GITHUB_TOKEN": "YOUR_TOKEN"
      }
    }
  }
}
```

#### Advanced — Using YAML config

For full control, create a `config.yaml` and pass it to `mcp-lens`:

```json
{
  "mcpServers": {
    "github-proxy": {
      "command": "/ABS/PATH/TO/mcp-lens",
      "args": ["-config", "/ABS/PATH/TO/config.yaml"],
      "env": {
        "GITHUB_TOKEN": "YOUR_TOKEN"
      }
    }
  }
}
```

### Cursor

Cursor supports MCP servers via a JSON config as well. Use the same stdio settings as Claude Desktop:

```json
{
  "mcpServers": {
    "github-proxy": {
      "command": "/ABS/PATH/TO/mcp-lens",
      "env": {
        "GITHUB_TOKEN": "YOUR_TOKEN"
      }
    }
  }
}
```

### VS Code

If you have an MCP extension / client that supports stdio servers, reuse the same command/args/env setup as shown above.

## How it works

- The proxy starts the upstream MCP server as a child process (`upstream.command` + `upstream.args`).
- It fetches upstream tools once (`tools/list`).
- It exposes only 3 meta-tools + tools you’ve activated.

Code pointers:
- Server entrypoint: `cmd/proxy/main.go`
- MCP server loop: `internal/server/server.go`
- Upstream process + JSON-RPC proxy: `internal/proxy/proxy.go`
- Meta-tools implementation: `internal/tools/meta.go`

## Security notes

- Your GitHub token is provided to the upstream server via environment variables.
- Prefer using a fine-scoped token.
- Don’t commit tokens or real config files with secrets.

## License

MIT — see `LICENSE`.
