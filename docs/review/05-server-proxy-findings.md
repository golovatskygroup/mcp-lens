# Review Findings: Server & Proxy (internal/server/*, internal/proxy/*)

## 1. Boundary Responsibilities
The current implementation suffers from "leaky abstractions" where transport-level concerns (JSON-RPC) are mixed with routing logic.

| Component | Current State | Target State (Smart Router) |
| :--- | :--- | :--- |
| **Server** (`internal/server`) | Handles both MCP transport and some tool logic. | **Pure Transport**: Only handles JSON-RPC framing, session lifecycle, and raw byte streams. |
| **Router** (`internal/router`) | Basic mapping of tool names to backends. | **Context Manager**: Performs Top-K tool selection, schema compression, and budget enforcement. |
| **Proxy** (`internal/proxy`) | Simple HTTP/SSE forwarding. | **Protocol Translator**: Manages upstream MCP client connections and enforces security policies. |

## 2. Identified Deficiencies

### 2.1 Context Inefficiency (High Priority)
- **Discovery Bloat**: The system currently returns all available tools from all backends. For large registries, this exceeds LLM context windows.
- **Schema Verbosity**: JSON schemas for tools are forwarded without compression, including redundant descriptions and nested objects.
- **Lack of Truncation**: No deterministic logic to truncate long tool outputs before they reach the LLM.

### 2.2 Error Handling & Logging
- **Non-Standard Errors**: Internal Go errors are sometimes leaked to the MCP client instead of being mapped to standard MCP/JSON-RPC error codes (e.g., `-32601` for Method not found).
- **Traceability**: Difficulty tracking a request from the Proxy through the Router to the specific Backend.

### 2.3 Security & Configuration
- **Tool Shadowing**: No protection against multiple backends registering the same tool name.
- **Resource Limits**: Lack of per-session memory and CPU limits for the proxying logic.

## 3. Improvement Plan (Phase 2.5)

### 3.1 Smart Routing & Context Saving
- [ ] **Static Registry**: Implement a `registry.json` or internal provider that returns a hardcoded list of "Core Tools" to avoid expensive discovery rounds.
- [ ] **Top-K Filtering**: Add a router middleware to filter the tool list based on the user's current intent (if available) or a priority score.
- [ ] **Schema Compression**: Implement a "Minifier" that strips `description` fields longer than 200 chars and removes optional metadata from JSON schemas.
- [ ] **Deterministic Truncation**: Enforce a `max_tokens` budget per tool response. If exceeded, the router must truncate the JSON and append a `[TRUNCATED]` marker.

### 3.2 Architectural Refactoring
- [ ] **Standardize `MCPError`**: Create a shared error package to ensure all components return valid JSON-RPC 2.0 error objects.
- [ ] **Context Propagation**: Ensure `context.Context` carries request IDs across the proxy-router boundary for unified logging.
- [ ] **Backend Isolation**: Move backend-specific logic into `internal/backend` to keep `internal/proxy` generic.

### 3.3 Scope Reduction (Context-Saving Focus)
- **Archive Duplicates**: Identify and move `.worktrees` and `stage*` directories to an `archive/` folder to prevent the agent from indexing redundant code.
- **Explicit Tooling**: Change the default behavior from `ListTools(all)` to `ListTools(filtered_by_registry)`.

## 4. Security Recommendations
- **Validation**: Implement strict JSON schema validation at the Proxy level before forwarding requests to the Router.
- **Authentication**: Add support for Bearer tokens in the SSE/HTTP transport layer of the Server.

---
*Status: Review Complete. Moving to implementation of Smart Router logic.*