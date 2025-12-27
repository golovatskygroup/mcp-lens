# mcp-lens

MCP proxy that saves your context window. One tool instead of hundreds.

## What it does

Your LLM sees ONE tool called `query`. You ask in plain English, mcp-lens figures out which upstream MCP tools to use.

No more 50+ tool schemas eating your tokens.

## Install

```bash
go install github.com/golovatskygroup/mcp-lens/cmd/proxy@latest
```

Or build from source:
```bash
git clone https://github.com/golovatskygroup/mcp-lens
cd mcp-lens
go build -o mcp-lens ./cmd/proxy
```

## Quick Start

```bash
# See available presets
mcp-lens --list-presets

# Interactive setup
mcp-lens setup

# Or just set env and run
export MCP_LENS_PRESET=github
export GITHUB_TOKEN=ghp_xxx
export OPENROUTER_API_KEY=sk-or-xxx
mcp-lens
```

## Presets

| Category | Servers |
|----------|---------|
| Reference | everything, time, memory, fetch, filesystem, git, puppeteer |
| Database | sqlite, postgres, redis, mongodb |
| Search | brave-search, tavily |
| Productivity | github, gitlab, slack, notion, linear |
| DevTools | sentry, aws, kubernetes |

Check if ready:
```bash
mcp-lens --check github
```

## Config

Environment variables:
```bash
MCP_LENS_PRESET=github
GITHUB_TOKEN=ghp_xxx
OPENROUTER_API_KEY=sk-or-xxx  # for routing
```

Or config file:
```yaml
# config.yaml
upstream:
  preset: github
```

```bash
mcp-lens --config config.yaml
```

## How it works

1. You call `query` with natural language
2. mcp-lens picks relevant tools from upstream MCP server
3. LLM plans and executes
4. You get compressed results

## Requirements

- Go 1.21+
- Node.js (for npx-based presets)
- OpenRouter API key (for routing logic)

## License

MIT
