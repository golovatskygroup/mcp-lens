# MCP Router: Review Process Template

## 1. Overview
This document defines the criteria and format for code and architecture reviews within the MCP Router project. The primary focus is on **context efficiency**, **deterministic tool selection**, and **scope-fit** to ensure the router remains lightweight and effective for LLM-based agents.

## 1.1 Project Goal: MCP Smart Router (Context-Saving)

The project implements **only** a smart MCP router that minimizes context usage by:
- Using explicit Tool Catalog instead of runtime tool discovery
- Selecting top-K most relevant tools per query
- Compressing tool schemas before including in prompts
- Enforcing strict token budgets and deterministic truncation

### Acceptance Criteria

(a) **No Runtime Tool Discovery**: Router does NOT use `search_tools`/`list_tools`/discovery tools in runtime. Tool list is provided explicitly via config/external registry.

(b) **Explicit Tool Catalog**: List of tools and their JSON schemas is provided explicitly (config file or inline in config), not discovered dynamically.

(c) **Top-K Tool Selection**: Only top-K most relevant tools (based on query) are included in LLM context, with compressed schemas.

(d) **Token Budget Compliance**: Router respects token/character budgets and performs deterministic truncation when limits are reached.

(e) **CI Tests Pass**: `go test ./...` passes in CI without requiring external services for unit tests.

---

## 2. Review Criteria

### 2.1 Architecture & Scope-Fit
- **Registry vs. Discovery**: Does the implementation use the explicit registry list instead of dynamic LLM tool discovery?
- **Modularity**: Is the logic for tool selection decoupled from the transport layer?
- **Scope Creep**: Does the change introduce features outside the "smart router" domain (e.g., complex state management that should be in the agent)?

### 2.2 Context Optimization (Critical)
- **Top-K Selection**: Is the deterministic selection of top-K tools implemented to minimize the prompt size?
- **Schema Compression**: Are tool schemas being stripped of redundant descriptions or metadata before being sent to the LLM?
- **Budgeting**: Does the code respect the token/character budget for tool definitions?
- **Truncation**: Is the truncation logic deterministic and predictable?

### 2.3 Security & Reliability
- **Input Validation**: Are tool arguments validated against schemas before proxying?
- **Error Handling**: Are MCP errors mapped to clean, context-saving messages for the LLM?
- **Isolation**: Does the router prevent one misbehaving MCP server from crashing the proxy?

### 2.4 Developer Experience (DX)
- **Type Safety**: Are Gleam's strong types used to prevent runtime errors in tool routing?
- **Observability**: Are there logs/traces for how the router decided to select specific tools?

### 2.5 Repository Hygiene
- **Duplicate Check**: Does this PR introduce files that overlap with existing `stage*` or `task*` directories?
- **Context Pollution**: Does the PR include unnecessary documentation or large binaries that would bloat the LLM's workspace context?

---

## 3. Findings Format

Each issue found during review must be recorded using the following structure:

| Field | Description |
| :--- | :--- |
| **ID** | `REV-[YYYYMMDD]-[NN]` |
| **Severity** | Critical / Major / Minor / Suggestion |
| **Category** | Context / Security / Architecture / DX / Hygiene |
| **Impact** | How this affects the system (e.g., "Increases context by 20%", "Potential crash") |
| **Finding** | Description of the deficiency |
| **Fix** | Proposed solution or requirement for approval |
| **Owner** | Person responsible for the fix |

---

## 4. Severity Definitions

- **Critical**: Must be fixed before merge. Blocks core functionality or violates context budget constraints.
- **Major**: Significant architectural or security concern. Requires a fix or a documented exception.
- **Minor**: Style issues, minor optimizations, or non-critical bugs.
- **Suggestion**: Ideas for future improvement; does not block merge.

---

## 5. Review Workflow

1.  **Self-Check**: Author verifies code against the "Context Optimization" criteria.
2.  **Submission**: PR is opened; automated tests and `gleam format` must pass.
3.  **Context Audit**: Reviewer specifically checks the `registry` and `schema` transformation logic.
4.  **Findings**: Reviewer fills out the Findings table in the PR comments.
5.  **Resolution**: Author addresses findings; Reviewer verifies and approves.
6.  **Post-Merge**: If the PR involved moving files from `stage*` folders, ensure the `scope-reduction-plan` is updated.

---

## 6. Findings Log (Example)

| ID | Severity | Category | Impact | Finding | Fix | Owner |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| REV-20251227-01 | Critical | Context | Context Bloat | Tool schemas are passed in full without compression. | Implement `compress_schema` function to strip descriptions. | @nyarum |
| REV-20251227-02 | Major | Hygiene | Context Pollution | PR includes duplicate logic found in `stage2/`. | Delete duplicate and reference the new router implementation. | @nyarum |