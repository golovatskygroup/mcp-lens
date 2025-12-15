# mcp-lens (mcp-proxy) — agent/dev notes

This repo is a small MCP proxy that exposes **only one public tool** (`query`) and internally routes/executes local + upstream tools under a **read-only policy**.

## Project layout (high-level)

- `internal/server/`: MCP JSON-RPC server. `tools/list` exposes only `query`.
- `internal/tools/`: local tools (GitHub helpers, Jira, Confluence, router runner, meta-tools).
- `internal/router/`: planning + validation + summarization logic, plus read-only allowlist policy.
- `internal/registry/`: tool registry (upstream + local schemas for discovery/routing).
- `.env.example`: documented env configuration (never commit real secrets in `.env`).

## How to add a new local tool (checklist)

Local tools are invoked as `source=local` steps in the router plan and are implemented on `*tools.Handler`.

1. **Implement the handler**
   - Add a new file in `internal/tools/` (or extend an existing domain file).
   - Implement `func (h *Handler) <toolMethod>(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error)`.
   - Parse/validate input strictly; return user-facing errors via `errorResult(...)`.
   - Return JSON via `jsonResult(...)` with a stable shape (maps/arrays, no raw bytes).

2. **Register the tool schema**
   - Add an entry in `internal/tools/meta.go` → `func (h *Handler) BuiltinTools() []mcp.Tool`.
   - Provide:
     - `Name`: stable snake_case name (prefer `domain_action_*`, e.g. `confluence_get_page`).
     - `Description`: short, task-oriented.
     - `InputSchema`: valid JSON Schema (as `json.RawMessage` string).

3. **Wire it into the dispatcher**
   - Add a `case` in `internal/tools/meta.go` → `func (h *Handler) Handle(...)`.
   - Add it to `internal/tools/meta.go` → `func (h *Handler) IsLocalTool(name string) bool`.

4. **Allow it for router execution (read-only)**
   - If the tool is safe/read-only and should be callable by `query`, add it to:
     - `internal/router/policy.go` → `DefaultPolicy().AllowLocal`
   - Keep names non-mutating: policy blocks tools whose names include `create/update/delete/write/...`.

5. **Make it discoverable (optional but recommended)**
   - If users should be able to find it via `search_tools`, add a summary entry in:
     - `internal/tools/meta.go` → `searchLocalTools(...)`
   - If it’s a new domain, extend:
     - `internal/tools/meta.go` → `expandQuery(...)` synonyms
     - `internal/router/prompt.go` instructions for the planner (domain workflow hints)

6. **Pagination support (if applicable)**
   - Prefer returning:
     - `has_next: true/false`
     - plus one of: `next_cursor`, `next_page`, `next_start`, `next_offset`
   - `internal/tools/router.go` has auto-continuation logic. If your pagination shape is new, extend
     `createContinuationStep(...)` to stitch args for the next call.

7. **Update env docs**
   - Add required env vars to `.env.example`.
   - If it changes user-facing configuration, update `README.md`.

8. **Format and run tests**
   - `gofmt -w ...`
   - `go test ./...`

## How to write E2E / integration tests

### Principles

- Default test suite (`go test ./...`) must be **offline** and deterministic.
- Any test that calls external services must be **opt-in** via build tags and must `t.Skip(...)` unless required env vars are present.
- Tests may rely on `.env` locally, but should not require it in CI; always check env vars explicitly.

### Tags used in this repo

- `//go:build integration`
  - Typically: router planning with OpenRouter + optional GitHub calls.
  - Requires env like `OPENROUTER_API_KEY`, `MCP_LENS_ROUTER_MODEL`, plus service-specific vars.
- `//go:build e2e`
  - Typically: direct external API calls (no LLM) against Jira/Confluence/etc.

### Env-driven fixtures (no hardcoded real repos/projects)

- GitHub PR URL for tests must come from:
  - `GITHUB_E2E_PR_URL=https://github.com/<owner>/<repo>/pull/<number>`
  - Use `internal/testutil.ParseGitHubPullRequestURL(...)` to extract `repo` + `number`.
- Jira issue key for tests must come from:
  - `JIRA_E2E_ISSUE_KEY=PROJ-123`
- Add new service fixtures following the same pattern: `FOO_E2E_*`, and document them in `.env.example`.

### .env loading in tests

Most packages include a `TestMain` that calls `internal/testutil.LoadDotEnv()` to load a local `.env` if present.
Do not assume `.env` exists; still guard tests with `t.Skip` when env is missing.

## General recommendations

- Keep `query` as the only public tool. Add capabilities via local tools + router policy allowlists.
- Prefer small, composable read-only tools over a single “do everything” endpoint.
- Treat response shapes as API contracts: evolve compatibly and keep fields stable.
- Avoid adding new dependencies unless necessary (repo is intentionally lightweight).

