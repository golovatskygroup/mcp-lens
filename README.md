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
Pick the asset matching your OS/arch (e.g. `darwin_arm64`, `linux_amd64`, `windows_amd64`).

## Configuration

### Quick Start — Environment Variables

The simplest way to run `mcp-lens` is by setting environment variables. The proxy will use sensible defaults for the upstream MCP server.

```bash
export GITHUB_TOKEN="your_github_token_here"
./mcp-lens
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

#### Advanced — Using npx wrapper

If you prefer to run the upstream server directly without the proxy defaults, you can use the npx wrapper approach:

```json
{
  "mcpServers": {
    "github-proxy": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-github"
      ],
      "env": {
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
