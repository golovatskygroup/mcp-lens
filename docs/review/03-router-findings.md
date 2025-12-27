# Core Router Review & Scope Reduction Plan (internal/router/*)

## 1. Current Architecture Review

### 1.1 Responsibilities
- **Registry**: Currently handles dynamic tool discovery. 
    - *Issue*: High latency and non-deterministic tool lists.
- **Dispatcher**: Routes requests to MCP servers.
    - *Issue*: Lacks strict budget enforcement for context windows.
- **Context Manager**: Handles truncation.
    - *Issue*: Truncation is often "naive" (simple tail-cutting) rather than semantic or budget-aware.

### 1.2 Identified Risks
- **Token Leakage**: Sending full JSON schemas for 50+ tools when only 3 are needed.
- **Non-Determinism**: LLM tool selection varies based on the order of tools in the prompt.
- **Context Bloat**: The presence of `stage*/` and `task*` directories in the worktree confuses the agent's own development context.

---

## 2. Scope Reduction & Optimization Strategy

### 2.1 Explicit Tool Registry (Context Saving)
**Change**: Transition from "LLM-driven discovery" to "Registry-driven injection".
- **Implementation**: The router will maintain a hardcoded or config-based mapping of `Intent -> ToolList`.
- **Benefit**: Reduces prompt size by 60-80% by excluding irrelevant tool definitions.

### 2.2 Deterministic Top-K Selection
**Change**: Implement a scoring mechanism for tool relevance.
- **Mechanism**: 
    1. Vector similarity (optional) or Keyword matching between User Query and Tool Description.
    2. Select Top-K (default K=5) tools.
    3. Compress JSON schemas (remove optional descriptions, shorten property names) before injection.

### 2.3 Strict Budgets & Truncation
**Change**: Move from "Best Effort" to "Hard Limit" context management.
- **Rules**:
    - **Tool Budget**: Max 2000 tokens for tool definitions.
    - **History Budget**: Max 4000 tokens for conversation history.
    - **Truncation Strategy**: Deterministic "Middle-Out" truncation (keep system prompt + latest turns).

---

## 3. Repository Cleanup (Context Reduction)

### 3.1 Duplicate/Stage Removal
The following directories are marked for archiving to prevent the agent from indexing redundant code:
- `stage1/`, `stage2/` -> Move to `archive/stages/`
- `.worktrees/*` -> Ensure excluded via `.gitignore` and `.mcpignore`.
- `task_logs/` -> Move to `.gitignore`.

---

## 4. Action Plan (Phase 2.3)

| Task | Description | Priority |
| :--- | :--- | :--- |
| **Registry Refactor** | Implement `GetToolsForIntent(intent string)` in `internal/router/registry.go`. | High |
| **Schema Compressor** | Add a utility to strip non-essential fields from MCP tool schemas. | Medium |
| **Budget Middleware** | Add a wrapper in `internal/router/router.go` to count tokens and truncate before calling LLM. | High |
| **Archive Script** | Create a shell script to move `stage*` directories to `archive/`. | Low |

## 5. Stability & Testing
- **Deterministic Tests**: Add table-driven tests in `router_test.go` ensuring that the same query always results in the same Top-K tool selection.
- **Leak Detection**: Integration tests to verify that `TotalTokens < MaxLimit` for all router outputs.