# Project Map & Scope Definition: Smart MCP Router

## 1. System Inventory
This map outlines the current subsystems in the repository and their roles.

| Subsystem | Description | Status in "Smart MCP Router" |
| :--- | :--- | :--- |
| **Router** | The core logic for receiving requests and routing them to the correct MCP server. | **In Scope (Core)** |
| **Registry** | A static or dynamic list of available tools and their metadata. | **In Scope (Core)** |
| **Discovery** | LLM-based or dynamic scanning of MCP servers to find tools. | **Out of Scope** (Moving to explicit lists) |
| **Proxy** | Transport layer handling MCP protocol (JSON-RPC) over SSE/Stdio. | **In Scope** |
| **Executor** | Logic that actually triggers the tool call on the target MCP server. | **In Scope** |
| **Tools** | Individual tool implementations and their JSON schemas. | **In Scope (Schema Compression)** |
| **Packages** | Shared Gleam/JS libraries for protocol handling. | **In Scope** |
| **Server** | The entry point for the MCP Router itself. | **In Scope** |

## 2. Scope: "Smart MCP Router" (Context-Saving Focus)
The primary objective is to minimize the token "tax" when an LLM interacts with multiple MCP servers.

### ✅ In-Scope (Focus Areas)
1.  **Explicit Tool Registry**: Instead of asking the LLM to "discover" tools, the router provides a pre-defined, explicit list of tools from a registry.
2.  **Deterministic Top-K Selection**: Implementation of a non-LLM mechanism (e.g., vector search or keyword matching) to select only the most relevant tools for the current prompt.
3.  **Schema Compression**: Stripping unnecessary descriptions or optional fields from JSON schemas before sending them to the LLM.
4.  **Strict Budgets & Truncation**: Deterministic rules for cutting off tool output or context once a token budget is reached.
5.  **Gleam Implementation**: Leveraging Gleam's type safety and BEAM's concurrency for the proxy/router logic.

### ❌ Out-of-Scope (Scope Reduction)
1.  **Dynamic LLM Discovery**: We will not use LLM calls to "browse" available servers.
2.  **Complex Multi-step Planning**: The router focuses on efficient tool *provisioning*, not autonomous agent planning.
3.  **Legacy Artifacts**: The `stage*` and `task*` directories and `.worktrees` are considered technical debt and are excluded from the target architecture.

## 3. Repository Findings & Cleanup Plan
The current repository contains significant context noise that must be addressed to maintain developer (and LLM) efficiency.

### Findings
*   **Duplicate Logic**: Multiple `stage_X` folders contain iterative versions of the same logic.
*   **Worktrees**: Active `.worktrees` folders are bloating the file tree and causing confusion in global searches.
*   **Mixed Targets**: Presence of both experimental JS and Erlang/BEAM targets without a unified build strategy.

### Context Reduction Strategy (Phase 2.2+)
1.  **Archive Stage Folders**: Move all `stage*` and `task*` directories into a single `/archive` folder or remove them if tracked in Git history.
2.  **Consolidate Core**: Move the active Gleam implementation of the router into a clean `/src` structure.
3.  **Ignore Worktrees**: Ensure `.gitignore` and LLM indexing configs (like `.cursorrules` or `.clinerules`) explicitly ignore `.worktrees`.

## 4. Decision Log

### What We're Removing/Deprecating Now

- **Runtime Tool Discovery**: `search_tools` and similar discovery tools are explicitly blocked. Router uses explicit `ToolCatalog` instead.
- **Dynamic Tool Introspection**: No runtime tool enumeration. Tools must be provided via config.

### What We're Keeping (Core Router)

- **ToolCatalog**: Explicit tool list with top-K selection and schema compression
- **Router Planning**: LLM-based planning with explicit tool catalog
- **Policy Enforcement**: Read-only policy with explicit discovery tool blocking
- **Schema Compression**: Deterministic schema summarization to save context

### What May Be Removed Later (Future Scope Reduction)

- **Discovery Package**: `internal/discovery/` may be deprecated if not needed for router-only use case
- **Executor Package**: `internal/executor/` may be isolated if router doesn't need code execution
- **Legacy Worktrees**: `.worktrees/` directories are snapshots and should be archived

## 5. Technical Stack
*   **Language**: Go (golang)
*   **Runtime**: Native Go binary
*   **Protocol**: MCP (Model Context Protocol) over JSON-RPC (stdio/SSE)
*   **Key Libraries**: Standard Go libraries + `gopkg.in/yaml.v3` for config