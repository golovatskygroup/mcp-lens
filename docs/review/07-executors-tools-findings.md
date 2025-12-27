# Project Review: Executors & Tools (2.7)

## 1. Current State Assessment

### 1.1 Components Reviewed
- `internal/executor/*`: Contains logic for running tasks, managing environments, and executing commands.
- `internal/tool/*`: Defines tool interfaces and basic tool implementations.
- `internal/tools/*`: (Note the plural) Often contains specific tool integrations and helper utilities.

### 1.2 Key Findings
- **Overlapping Responsibilities**: There is no clear boundary between an "Executor" (which runs code) and a "Tool" (which provides a capability). This leads to duplicate logic for error handling and timeout management.
- **Context Bloat**: Tools are currently registered with full JSON schemas. In a high-tool-count environment, this exceeds LLM context windows.
- **Legacy Artifacts**: The presence of `stage*` and `task*` logic suggests a workflow engine that is no longer the primary focus of the "Smart MCP Router."
- **Discovery Overhead**: The system relies on the LLM to "find" tools, rather than the router "offering" the most relevant ones.

---

## 2. Isolation & Deletion Plan

To reduce context and focus on the MCP Router goals, the following structural changes are mandated:

### 2.1 Archive/Deprecation (Immediate Isolation)
- **Move to `internal/archive/executor`**: All logic related to multi-stage task execution (`stage_manager.go`, `task_graph.go`). The router only needs to execute *single* tool calls.
- **Remove `internal/tools/legacy_*`**: Any hardcoded tools that are now available via MCP servers should be deleted to force usage of the MCP protocol.

### 2.2 Unification (The "Lean" Registry)
- **Consolidate into `internal/registry`**:
    - Create a single source of truth for all available tools (Local + MCP).
    - Implement the **Top-K Deterministic Selector**: Instead of sending 100 tools to the LLM, the registry will use vector embeddings or keyword matching to send only the top 5-10.
    - Implement **Schema Compression**: Strip descriptions and optional fields from JSON schemas before sending them to the LLM, providing the full schema only upon execution.

### 2.3 Refactoring `internal/tool`
- **Interface Simplification**: The `Tool` interface should be reduced to a single `Execute(ctx, args) (result, error)` method.
- **MCP-First**: `internal/tool/mcp_proxy.go` becomes the primary implementation. All other tools (shell, file-read) should be wrapped to look like MCP tools to maintain a uniform internal API.

---

## 3. Scope Reduction Strategy

| Feature | Status | Action |
| :--- | :--- | :--- |
| **LLM Tool Discovery** | Removed | Replaced by Registry-side deterministic filtering. |
| **Full Schema Passing** | Removed | Replaced by "Just-in-Time" schema hydration. |
| **Multi-step Executors** | Archived | Moved to `internal/archive`. |
| **Deterministic Truncation** | New | Strict token budgets for tool outputs in `internal/executor`. |

---

## 4. Implementation Roadmap (Phase 3)

1. **Step 1**: Create `internal/registry` and migrate tool definitions from `internal/tool`.
2. **Step 2**: Implement a `Compact()` method for tool schemas to reduce token footprint by ~60%.
3. **Step 3**: Relocate `internal/executor` logic that handles "retries" and "state" into a simplified `ToolRunner` that respects strict context budgets.
4. **Step 4**: Physical deletion of `.worktrees` and duplicate `stage*` files to clean up the repository footprint.

## 5. Decision Log
- **Decision**: We will not support "Autonomous Agents" in this scope.
- **Reasoning**: To save context and ensure reliability, we are building a **Smart Router**, not a general-purpose agent. The router decides *which* tool to call and *how* to format the response, keeping the LLM in a "pure reasoning" loop.