# Cursor AI Rules — mcp-lens

This file contains rules and instructions for Cursor AI when working with this repository.

## Task Management System

### Task Storage

Tasks are stored in `.tasks/` directory with the following structure:

```
.tasks/
├── active/           # Tasks currently in progress
│   └── TASK-XXX-*.md
├── completed/        # Finished tasks (archive)
│   └── TASK-XXX-*.md
└── backlog/          # Planned but not started
    └── TASK-XXX-*.md
```

### Task File Format

Each task file follows this template:

```markdown
# Задача: <Short Title>

## Описание
<1-3 sentences explaining the goal>

## Контекст
- **Документация:** <relevant docs>
- **Целевые файлы:** <files to create/modify>
- **Зависимости:** <prerequisite tasks if any>

## TODO

### Phase N: <Phase Name>
- [ ] **N.1** <Task description>
  - <Details/subtasks>
  - Файл: `path/to/file.go`
- [ ] **N.2** ...
- [x] **N.3** <Completed task>

## Статус
- **Создана:** YYYY-MM-DD
- **Обновлена:** YYYY-MM-DD
- **Статус:** В работе | Завершена | Отложена

## Заметки
<Additional context, decisions, blockers>

## Критерии приемки
- [ ] <Acceptance criterion 1>
- [ ] <Acceptance criterion 2>
```

### Task Naming Convention

Task files follow the pattern: `TASK-XXX-<short-slug>.md`

- `XXX` — sequential 3-digit number (001, 002, ...)
- `<short-slug>` — kebab-case description, max 40 chars

Examples:
- `TASK-001-full-project-review.md`
- `TASK-002-router-entrypoint.md`
- `TASK-003-add-mcp-presets.md`

### Working with Tasks

#### Creating a New Task

1. Find the next available task number in `.tasks/`
2. Create file in `.tasks/active/TASK-XXX-<slug>.md`
3. Fill in the template with:
   - Clear description and goal
   - Phased TODO list with checkboxes
   - Target files for each item
   - Acceptance criteria

#### Updating Task Progress

When working on a task:
1. Mark completed items with `[x]`
2. Update the `Обновлена` date
3. Add notes about decisions/blockers

#### Completing a Task

1. Ensure all TODO items are checked `[x]`
2. Ensure all acceptance criteria are met
3. Update status to `Завершена`
4. Move file to `.tasks/completed/`

### Task Commands

Use these commands to manage tasks:

```bash
# List active tasks
ls -la .tasks/active/

# Create new task (use template)
cp .tasks/TEMPLATE.md .tasks/active/TASK-XXX-new-task.md

# Move completed task to archive
mv .tasks/active/TASK-XXX-*.md .tasks/completed/

# Find task by keyword
grep -r "keyword" .tasks/
```

### Integration with Development

When starting work:
1. Check `.tasks/active/` for current priorities
2. Pick a task or create new one for the request
3. Update task file as you progress
4. Commit task file changes with code changes

When completing work:
1. Verify acceptance criteria
2. Run tests: `go test ./...`
3. Run linter: `golangci-lint run`
4. Update task status
5. Commit and push

---

## Project-Specific Rules

### Code Style

- Go code must pass `gofmt` and `golangci-lint`
- Use `snake_case` for tool names (e.g., `github_get_pr`)
- Use `camelCase` for Go function names
- JSON responses must be stable (deterministic field order)

### Testing Requirements

- Unit tests: `go test ./...` must pass offline
- Integration tests: use `//go:build integration` tag
- E2E tests: use `//go:build e2e` tag
- Never hardcode real credentials/URLs in tests

### Adding New Presets

When adding MCP server presets to `internal/presets/presets.go`:

1. Add preset to `NewRegistry()` map
2. Add preset name to `ListAvailable()`
3. Add test in `presets_test.go`
4. Document env vars in `.env.example`

### Adding New Tools

Follow the checklist in `AGENTS.md`:

1. Implement handler in `internal/tools/`
2. Register schema in `meta.go` → `BuiltinTools()`
3. Wire dispatcher in `meta.go` → `Handle()`
4. Add to policy if read-only: `internal/router/policy.go`
5. Update `.env.example` with required env vars
6. Add tests

### Commit Messages

Use conventional commits:
- `feat:` — new feature
- `fix:` — bug fix
- `docs:` — documentation
- `test:` — tests
- `refactor:` — code refactoring
- `chore:` — maintenance

Example: `feat: add postgres and slack presets`

---

## Quick Reference

### Build & Test

```bash
make build      # Build all binaries
make test       # Run unit tests
make lint       # Run linter
make fmt        # Format code
go build ./...  # Build (alternative)
go test ./...   # Test (alternative)
```

### Key Directories

| Directory | Purpose |
|-----------|---------|
| `cmd/proxy/` | Main entrypoint |
| `internal/server/` | MCP server |
| `internal/router/` | Smart routing logic |
| `internal/tools/` | Built-in tools |
| `internal/presets/` | Server presets |
| `pkg/mcp/` | MCP protocol |
| `packages/` | External packages (Python, etc.) |
| `.tasks/` | Task management |
| `docs/` | Documentation |

### Environment Variables

Key env vars (see `.env.example` for full list):

| Variable | Purpose |
|----------|---------|
| `OPENROUTER_API_KEY` | Router LLM API key |
| `MCP_LENS_PRESET` | Preset name (github, search, etc.) |
| `GITHUB_TOKEN` | GitHub API access |
| `MCP_LENS_ROUTER_MODEL` | LLM model for routing |
