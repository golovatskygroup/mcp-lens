# MCP Layer Review: Protocol, Transport, and Context Optimization

## 1. Current State Assessment (pkg/mcp/*)

### 1.1 Protocol Completeness
- **Status**: Partial implementation of the Model Context Protocol (MCP).
- **Strengths**: Basic JSON-RPC 2.0 transport is functional; handshake/initialization follows the spec.
- **Weaknesses**: 
    - Missing robust implementation of `resources/read` and `resources/list`.
    - Tool definitions are currently passed "as-is" to the LLM, leading to high token overhead for complex schemas.
    - Lack of explicit support for `notifications` (e.g., progress updates).

### 1.2 Transport & Reliability
- **Timeouts**: Hardcoded 30s timeouts across all requests. No differentiation between fast (list_tools) and slow (tool_call) operations.
- **Retries**: No built-in retry logic for transient transport errors (EOF, connection reset).
- **Concurrency**: Basic mutex locking on the registry; potential for head-of-line blocking on slow tool executions.

---

## 2. Identified Deficiencies

| Category | Deficiency | Impact |
| :--- | :--- | :--- |
| **Context** | LLM-based tool discovery sends the entire toolset every time. | High token cost; context window exhaustion. |
| **Determinism** | No mechanism to select only the "Top-K" relevant tools before the LLM call. | LLM hallucination on tool selection; increased latency. |
| **Contracts** | Tool schemas are verbose (OpenAPI-style) and contain redundant metadata. | Waste of 20-40% of prompt space. |
| **Resilience** | Lack of circuit breakers for failing MCP servers. | One slow server can hang the entire proxy. |

---

## 3. Improvement Plan: Smart MCP Router

### 3.1 Context-Saving Optimizations (Priority 1)
- **Explicit Registry Filtering**: Replace LLM tool discovery with a deterministic `Registry.GetTools(tags []string)` method.
- **Top-K Selection**: Implement a local embedding-based or keyword-based pre-filter to select only the most relevant tools (e.g., limit to 5-10 tools per turn).
- **Schema Compression**:
    - Strip `description` fields from sub-properties if the parent property is self-explanatory.
    - Remove `example` and `default` fields from the schema sent to the LLM (keep them in the registry for validation).
- **Deterministic Truncation**: Implement a "Token Budget" for tool outputs. If a tool returns 50KB of JSON, the router must truncate it based on a strict budget before passing it back to the LLM.

### 3.2 Protocol & Transport Enhancements
- **Adaptive Timeouts**:
    - `initialize`: 5s
    - `list_tools`: 2s (cached)
    - `call_tool`: 60s (configurable per tool)
- **Retry Strategy**: Implement exponential backoff (max 3 retries) for `JSON-RPC` transport errors.
- **Health Checks**: Background heartbeat to MCP servers to mark them as "offline" in the registry if they fail to respond.

---

## 4. Implementation Tasks (TODO 2.4)

- [ ] **Refactor `pkg/mcp/registry`**: Add support for tool tagging and metadata-based filtering.
- [ ] **Implement `pkg/mcp/compressor`**: Create a utility to minify JSON schemas for LLM prompts.
- [ ] **Update `pkg/mcp/client`**: 
    - Add `context.Context` propagation with per-call timeouts.
    - Add middleware support for logging and retry logic.
- [ ] **Budget Manager**: Create a `pkg/mcp/budget` package to track and enforce token limits on tool responses.

---

## 5. Note on Project Structure (Scope Reduction)
The review identified significant duplication in `stage*/task*` directories and `.worktrees`. 
- **Action**: These will be ignored by the MCP router's context-gathering logic to prevent the LLM from seeing multiple versions of the same codebase.
- **Future**: Archive these directories into a single `_archive/` folder to reduce the physical file count and improve `grep/find` performance during development.