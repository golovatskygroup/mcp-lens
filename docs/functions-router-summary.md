# Summary: Attempt to discover GitHub operations via `functions.router`

## Context

You asked me to use `functions.router` to determine which GitHub operations are available (e.g., issues/PR reads, searches, file browsing, branches) and how to invoke them.

This summary documents what I tried, what happened, and what that implies for debugging/UX.

## What I attempted

I invoked `functions.router` with an instruction to:

- identify any GitHub-related operations available in this environment
- list concrete actions/endpoints (e.g., issue/PR list/read/search, file contents, branches/tags, releases, users/teams)
- explain how to call them

In other words: use the router as a “tool discovery” mechanism, not to perform a specific GitHub action.

### Router request payload (exact)

    {
      "input": "Проверь, какие операции с GitHub доступны через functions.router в этом окружении (если есть): например, чтение issues/PR, поиск, просмотр файлов/веток и т.д. Перечисли конкретные действия/эндпоинты, которые ты можешь выполнить, и как их вызвать.",
      "max_steps": 6,
      "include_answer": true,
      "format": "text"
    }

## Observed result

The `functions.router` call did not complete successfully.

The router attempted to perform an internal “tool discovery” step and was blocked by policy.

### Router error output (exact)

    Error: tool blocked by policy: search_tools
    plan={
      "steps": [
        {
          "name": "search_tools",
          "source": "local",
          "args": {
            "query": "",
            "format": "json",
            "include_schemas": true,
            "limit": 100
          },
          "reason": "Для начала нужно получить полный список всех доступных инструментов (функций) в формате JSON, включая их схемы, чтобы точно определить, какие операции чтения доступны, и как их вызывать. Установлен лимит 100, чтобы получить все доступные инструменты."
        }
      ],
      "final_answer_needed": true
    }

The error message indicates:

- the call was rejected due to policy restrictions
- the blocked step was a local tool-enumeration / tool-search operation
- as a result, the router could not proceed to enumerate or describe any GitHub-related capabilities

This means that, in this environment, `functions.router` is **not usable for discovering tool surfaces** when its plan requires that internal tool-search/list functionality.

## Implications

1. **Router cannot be used as a capability introspection mechanism here**
   - If the router relies on internal tool listing/searching, it will fail before it can report what’s available.

2. **The failure happens before any GitHub-specific action**
   - The block occurred at the discovery phase, not at an authorization phase tied to GitHub itself.

3. **User-facing symptom**
   - From the user’s perspective: “I asked router what GitHub operations exist; router errors out immediately.”

4. **Why this matters for GitHub MCP usage**
   - Tool discovery is essential to safely call GitHub operations because you need the exact tool names and argument schemas.
   - When discovery is blocked, the only remaining options are:
     - rely on out-of-band documentation/configuration, or
     - hardcode assumptions about tool names/schemas (unsafe / brittle).

## Recommended next steps for the developer

- **Allow discovery in a controlled, read-only way**
  - Ensure that whatever “tool discovery” mechanism the router uses is permitted (at least in read-only mode), so it can enumerate and describe tools safely.

- **Provide an explicit non-router path to introspection**
  - If router-based introspection is intentionally restricted, expose a safe way to:
    - list available tools
    - fetch a single tool’s schema
  - This avoids relying on router planning behavior.

- **Improve error messaging**
  - The error should clearly communicate:
    - that discovery is blocked by policy
    - what alternative path the user should use to list/describe tools

## Reproduction notes

- Trigger: request the router to enumerate/describe available GitHub operations.
- Result: policy block at the tool-discovery phase (before any GitHub action occurs).

---

## Resolution (TASK-002 Implementation)

**Status**: ✅ **FIXED**

The router has been updated to use an **explicit Tool Catalog** instead of runtime tool discovery. This resolves the policy block issue while maintaining context efficiency.

### How It Works Now

1. **Explicit Tool Catalog**: Tools are loaded from configuration (inline or file) rather than discovered at runtime. The catalog is provided to the router during initialization.

2. **Top-K Selection**: The router uses `ToolCatalog.TopKByQuery()` to select only the most relevant tools for each query, minimizing context usage.

3. **Schema Compression**: Tool schemas are compressed using `SummarizeSchema()` before being included in prompts, reducing token usage.

4. **Policy Enforcement**: The router explicitly blocks discovery tools (`search_tools`, `list_tools`, etc.) in both policy checks and plan validation, ensuring no runtime discovery occurs.

### How to Debug Tool Availability

Instead of using `search_tools` (which is blocked), use one of these approaches:

1. **Check Configuration**: Review `config.example.yaml` or your actual config file to see which tools are included in the catalog.

2. **Use Tool Catalog API**: The `ToolCatalog` provides:
   - `ListTools()`: Get all tools in the catalog
   - `GetTool(name)`: Get a specific tool by name
   - `TopKByQuery(query, k)`: Find most relevant tools for a query

3. **MCP `tools/list`**: The MCP server exposes available tools via the standard `tools/list` method (this is separate from router planning).

### Configuration Example

```yaml
router:
  tool_catalog:
    source: "inline"  # or "file" to load from JSON file
    inline_tools: []  # Tools defined here
  top_k_tools: 10     # Number of tools to include in prompt
  schema_budget: 800  # Max characters per schema
```

### Error Messages

If a plan attempts to use discovery tools, you'll now see:
```
invalid plan: tool discovery/introspection (search_tools) is not allowed; 
router uses explicit ToolCatalog instead of runtime discovery
```

This clearly indicates that discovery tools are intentionally blocked and directs users to use the explicit catalog approach.
