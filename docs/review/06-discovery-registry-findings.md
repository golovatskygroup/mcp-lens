# Review Findings: Discovery & Registry (Item 2.6)

## 1. Current State Assessment
The existing `internal/discovery` and `internal/registry` modules contain logic for dynamic server probing and broad tool indexing. While robust, this approach is "context-heavy" because it often passes large, unoptimized tool schemas to the LLM, relying on the model to "discover" what it needs.

### Redundancies & Over-engineering:
- **Dynamic Discovery**: Probing unknown MCP endpoints at runtime adds latency and unpredictable context growth.
- **Full Schema Passing**: Passing entire JSON-RPC schemas for every available tool consumes tokens unnecessarily.
- **Stage/Task Duplicates**: Fragmented registry logic across `stage*` and `task*` directories creates maintenance overhead.

### Cost & Dependency Analysis:
- **Latency**: Dynamic probing adds ~200ms-1s per request.
- **Token Usage**: Full schemas consume 1k-5k tokens.
- **Dependencies**: Removing `github.com/mcp-server/probe-lib` and similar dynamic scanners. The minimal variant uses only standard library `sync.Map` and `strings`.

## 2. Target Architecture: Smart MCP Router
To meet the goal of context-saving and deterministic routing, the discovery/registry logic is reduced to a **Static-First Registry**.

### Key Focus Areas:
1.  **Explicit Tool Manifests**: Tools are loaded from a static YAML/JSON manifest at startup.
2.  **Deterministic Top-K Selection**: A local keyword-based matcher selects the most relevant $K$ tools.
3.  **Schema Compression**: Registry entries store "minified" versions of tool definitions.

## 3. Minimal Necessary Implementation

### `internal/registry` (The Core)
The registry is reduced to a simple in-memory lookup and filtering service.