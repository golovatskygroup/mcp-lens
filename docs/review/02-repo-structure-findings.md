# Repository Structure Review & Context Reduction Plan

## 1. Executive Summary
The current repository structure of the `mcp-proxy` project contains significant "context bloat" due to historical task tracking, multiple staging directories, and git worktrees. This overhead increases the token count for LLMs and complicates project understanding. To align with the "Smart MCP Router" scope, we must prune non-essential files and consolidate the core logic.

## 2. Findings: Context Bloat & Risks

### 2.1 Redundant Directories
- **`stage*/` and `task*/` directories**: These appear to be snapshots of progress or isolated development environments. They contain duplicates of core logic, which confuses LLM tool discovery and increases the risk of referencing outdated code.
- **`.worktrees/`**: While useful for local development, including worktrees in the primary context (or not ignoring them in `.mcpignore`) leads to massive duplication of the entire codebase.
- **`archive/`**: Contains legacy scripts that are no longer part of the deterministic router logic.

### 2.2 Maintenance Risks
- **Version Drift**: Fixes applied in a `task/` directory might not be ported to the main `src/`, leading to fragmented "sources of truth."
- **LLM Hallucination**: When an LLM sees three versions of `router.gleam`, it may mix implementation details from different stages.
- **Registry Confusion**: If tools are defined in multiple staging areas, the deterministic registry selection becomes non-deterministic due to path ambiguity.

## 3. Recommendations

### 3.1 Immediate Deletion (High Impact)
- **Remove all `.worktrees/`**: These should never be part of the project's permanent structure. Use a global `.gitignore` or move them outside the project root.
- **Delete completed `task*/`**: Once a task is merged into the main branch or current stage, the task-specific directory should be removed.

### 3.2 Archival (Externalize)
- **Move `stage1/` through `stageN-1/`**: Move historical stages to a separate `legacy-archive` repository or a compressed `archive.tar.gz` that is ignored by the MCP server.
- **Documentation Snapshots**: Keep only the latest `docs/gleam/*.md` files. Move older versions to a `docs/archive/` folder and add it to `.mcpignore`.

### 3.3 Structural Consolidation
- **Single Source of Truth**: All active Gleam code must reside in `src/`.
- **Registry Centralization**: Move all tool definitions into a single `src/registry/` directory to facilitate the "explicit list of tools" requirement.

## 4. Implementation Plan (Post-Approval)

| Action Item | Description | Target Date |
| :--- | :--- | :--- |
| **Step 1: Pruning** | Delete `.worktrees` and redundant `task/` folders. | After PR Approval |
| **Step 2: Archiving** | Move `stage*` folders to an external location. | After PR Approval |
| **Step 3: MCP Ignore** | Update `.mcpignore` to strictly include only `src/`, `gleam.toml`, and active `docs/`. | Immediate |
| **Step 4: Registry Fix** | Consolidate tool schemas into `src/registry/tools.gleam`. | Phase 2.3 |

## 5. Impact on Smart MCP Router
By reducing the repository to its essential components, we achieve:
1. **Deterministic Discovery**: LLMs only see the current registry.
2. **Lower Latency**: Smaller context windows lead to faster LLM processing.
3. **Strict Budgets**: Truncation logic becomes easier to implement when the "base" context is predictable.

---
*Review conducted as part of TODO Item 2.2 - Phase 2.*