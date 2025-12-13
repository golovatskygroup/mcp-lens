# mcp-lens (GitHub)

A tiny **MCP proxy** that wraps an upstream MCP server (currently: GitHub) and exposes a *smaller*, easier tool surface to the client.

Instead of loading every upstream tool into your client, this proxy starts the upstream server and provides **3 meta-tools**:
- `search_tools` — find available tools by keyword/category
- `describe_tool` — get a tool schema/description
- `execute_tool` — execute a discovered tool

As you use `execute_tool`, the proxy **auto-activates** the underlying tools for the current session.

> Status: **GitHub backend is implemented** (via `@modelcontextprotocol/server-github`).
> More upstreams/adapters can be added later.

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
go build -o mcp-github-proxy ./cmd/proxy
```

### Option B — Use the prebuilt binary

This repo currently contains a local binary named `mcp-github-proxy` (macOS build).
For a public release, prefer building yourself or publishing GitHub Releases per platform.

## Configuration

The proxy can run with a default upstream (GitHub MCP via `npx`), or you can pass a config file.

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

Run:

```bash
./mcp-github-proxy -config ./config.example.yaml
```

## Add to your editor / client

Below are minimal MCP stdio examples.

### Claude Desktop

Add a server entry pointing to the binary.

```json
{
  "mcpServers": {
    "github-proxy": {
      "command": "/ABS/PATH/TO/mcp-github-proxy",
      "args": ["-config", "/ABS/PATH/TO/config.example.yaml"],
      "env": {
        "GITHUB_TOKEN": "YOUR_TOKEN"
      }
    }
  }
}
```

Create your own `config.yaml` locally (do **not** commit it), or use the example config as-is.

### Cursor

Cursor supports MCP servers via a JSON config as well. Use the same stdio settings:

```json
{
  "mcpServers": {
    "github-proxy": {
      "command": "/ABS/PATH/TO/mcp-github-proxy",
      "args": ["-config", "/ABS/PATH/TO/config.example.yaml"],
      "env": {
        "GITHUB_TOKEN": "YOUR_TOKEN"
      }
    }
  }
}
```

### VS Code

If you have an MCP extension / client that supports stdio servers, reuse the same command/args/env.

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
