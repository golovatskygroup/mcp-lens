# Task 0007: Dev-time генератор локальных tools через LLM planner (patch + отдельный worktree)

## Зачем

Команде нужно быстро добавлять новые **локальные read-only tools** (например, `jira_export_*`, `confluence_get_*_text`, `grafana_*`) без рутины:
- написать handler,
- добавить JSON Schema в `BuiltinTools`,
- прописать dispatch в `Handle(...)`,
- добавить `IsLocalTool(...)`,
- (опционально) добавить в `searchLocalTools(...)`, `expandQuery(...)`, `prompt.go`,
- (опционально) добавить в allowlist политики `internal/router/policy.go`.

Хочется dev-time “генератор”, который по запросу генерирует **патч/скелет** и сохраняет его локально (в repo), чтобы разработчик мог:
- посмотреть diff,
- применить руками (`git apply`) и доработать,
- прогнать `gofmt`/`go test`.

Важно: `query` остаётся **единственным публичным tool**.

## Ключевая идея (MVP)

Добавить локальный tool `dev_scaffold_tool`, который:
- принимает структурированное описание будущего tool (имя, описание, schema, домен),
- генерирует **patch file** и (опционально) создаёт **отдельный git worktree** и применяет патч туда,
- не делает `git commit`/`push`,
- по умолчанию работает только при `MCP_LENS_DEV_MODE=1`.

MVP сфокусирован на генерации **локальных tools** (source=local), без runtime-динамической регистрации. Сборка/тесты опциональны.

## UX (как будет использоваться)

### Вариант B (через LLM planner) — рекомендуемый

Разработчик пишет в MCP-клиенте free-form запрос вроде:
- “Добавь локальный tool `confluence_get_page_text` (read-only), который возвращает чистый текст из body.storage/view; добавь в policy allowlist.”

Router planner генерирует план и вызывает `dev_scaffold_tool`, который:
- пишет патч в `tasks/scaffolds/<tool_name>.patch`,
- создаёт worktree (например, `.worktrees/dev-<tool_name>-<ts>`),
- применяет патч внутри worktree,
- (опционально) прогоняет `gofmt`/`go test`.

Плюсы:
- не требует “командного” синтаксиса: работает из обычного запроса;
- патчи и изменения изолированы в отдельном worktree → удобно ревьюить/дорабатывать;
- patch-ориентированный flow хорошо ложится на git review.

### Вариант A (fast-path) — fallback

Для детерминизма/без ключей можно оставить fast-path (`scaffold tool ...`) как fallback, но MVP ориентируем на planner-first.

## Контракт нового tool (предложение)

### Name

`dev_scaffold_tool` (важно: не содержит `create_`/`update_`/`delete_`/`write`, иначе будет заблокирован `isMutatingName`).

### InputSchema (MVP)

```json
{
  "type": "object",
  "properties": {
    "tool_name": { "type": "string", "description": "snake_case имя (например confluence_get_page_text)" },
    "tool_description": { "type": "string" },
    "input_schema": { "type": "object", "description": "JSON Schema объекта arguments" },
    "handler_method": { "type": "string", "description": "Go method name (если пусто — вычислить из tool_name)" },
    "domain": { "type": "string", "description": "jira|confluence|grafana|github|meta|other (для выбора файла/пакета)" },
    "target_dir": { "type": "string", "description": "Куда положить patch (default: tasks/scaffolds/)" },
    "use_worktree": { "type": "boolean", "description": "Создать отдельный worktree и применить patch туда", "default": true },
    "worktree_root": { "type": "string", "description": "Корень для worktrees (default: .worktrees/)" },
    "worktree_name": { "type": "string", "description": "Имя worktree директории (если пусто — auto dev-<tool>-<ts>)" },
    "run_gofmt": { "type": "boolean", "description": "Запустить gofmt в worktree", "default": true },
    "run_tests": { "type": "boolean", "description": "Запустить go test ./... в worktree", "default": false },
    "allow_in_policy": { "type": "boolean", "description": "Добавить ли в internal/router/policy.go allowlist", "default": false },
    "add_to_search": { "type": "boolean", "description": "Добавить ли в searchLocalTools", "default": true },
    "add_prompt_hint": { "type": "boolean", "description": "Добавить ли подсказку в router/prompt.go", "default": false }
  },
  "required": ["tool_name", "tool_description", "input_schema"]
}
```

### Output (MVP)

JSON с устойчивой формой:
- `ok: true`
- `patch_path: "tasks/scaffolds/<tool_name>.patch"`
- `worktree_path: ".worktrees/dev-<tool_name>-<ts>"` (если `use_worktree=true`)
- `files_touched: []string` (пути, которые патч изменит)
- `next_steps: []string` (инструкции: `git apply`, `gofmt`, `go test`)
- `warnings: []string` (например: “tool_name уже существует”)

## Где и как генерировать патч (предложение)

### Генерируемые изменения (минимум)

1) `internal/tools/<domain>_*.go` (или новый файл `internal/tools/<tool_name>.go`):
   - `type <ToolInput> struct {...}` (только input struct; без бизнес-логики)
   - `func (h *Handler) <handler_method>(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error)`:
     - `json.Unmarshal` + строгая валидация required
     - `return errorResult(...)` на ошибки
     - `return jsonResult(map[string]any{"ok": true, "todo": "implement"})` как заглушка

2) `internal/tools/meta.go`:
   - добавить tool schema в `BuiltinTools()`
   - добавить `case` в `Handle(...)`
   - добавить в `IsLocalTool(...)`
   - (опционально) добавить `searchLocalTools(...)` + `expandQuery(...)` synonym

3) `internal/router/policy.go`:
   - (опционально) добавить tool в `DefaultPolicy().AllowLocal`

4) `internal/router/prompt.go`:
   - (опционально) добавить инструкцию для planner

### Вывод патча

Патч сохраняем как обычный unified diff, который применим `git apply`:
- `tasks/scaffolds/<tool_name>.patch`

Примечание: генератор должен быть deterministic:
- сортировать JSON schema keys (или pretty-print),
- использовать фиксированный шаблон.

## Безопасность и ограничения

Обязательные:
- `MCP_LENS_DEV_MODE=1` иначе tool возвращает `isError=true` (“dev mode disabled”).
- Запись только в поддерево `tasks/` (или явно разрешённый список директорий).
- Санитизация путей и имён: `tool_name` только `[a-z0-9_]+`, без `..`, без слешей.
- Никаких сетевых вызовов внутри генератора (planner уже использует OpenRouter).
- Никаких изменений “втихаря”: результат — патч + (опционально) отдельный worktree; никаких коммитов/пушей.
- Worktree путь только под `.worktrees/` и только внутри git repo (`git rev-parse --show-toplevel`).

Опциональные:
- `dry_run=true` — вернуть patch в ответе без записи файла.
- `overwrite=true` — перезаписать существующий patch.

## План реализации (для команды)

### Этап 1 — MVP scaffolder + worktree apply (1–2 дня)

1) Реализовать handler `dev_scaffold_tool`:
   - парсер/валидация input
   - генерация patch string из шаблонов
   - запись в `tasks/scaffolds/`
   - (опционально) `git worktree add` в `.worktrees/` + `git apply` патча внутри worktree
   - (опционально) `gofmt`/`go test` в worktree
   - stable JSON output

2) Регистрация в `internal/tools/meta.go`:
   - `BuiltinTools()` schema
   - `Handle(...)` dispatch
   - `IsLocalTool(...)`
   - `searchLocalTools(...)` + `expandQuery(...)` (“scaffold”, “generate tool”, “template”)

3) Политика:
   - Добавить tool в `DefaultPolicy().AllowLocal`, но обязательно gate внутри tool через `MCP_LENS_DEV_MODE=1`.
   - Дополнительно: скрывать tool из router catalog, если dev mode выключен (чтобы planner случайно не выбирал).

4) Prompt hints в `internal/router/prompt.go`:
   - объяснить planner, что при запросах “add new local tool / scaffold tool / generate tool” нужно вызывать `dev_scaffold_tool` и включать `use_worktree=true`.

### Этап 2 — Тесты (0.5–1 день)

Offline unit tests:
- sanitizer tool_name/path
- генерация patch содержит ожидаемые секции (`meta.go` + новый файл)
- dev-mode gating: без env должен возвращать error
- worktree flow: не обязательно реально запускать git в тестах (можно замокать runner/exec слой), но проверить, что paths формируются безопасно.

### Этап 3 — Документация (0.5 дня)

- `.env.example`: добавить `MCP_LENS_DEV_MODE`
- `README.md`: раздел “Dev scaffolding”
- `initialize.instructions`: короткая подсказка, что такой tool существует в dev mode

## Acceptance criteria

- В dev mode (`MCP_LENS_DEV_MODE=1`) можно вызвать `query` с “scaffold tool …” и получить `patch_path`.
- Патч содержит все обязательные изменения (handler + meta wiring).
- В non-dev mode tool детерминированно отказывает и ничего не пишет на диск.
- `go test ./...` остаётся offline и проходит.

## Открытые вопросы (решить до кода)

1) Добавлять ли `dev_scaffold_tool` в allowlist policy по умолчанию, или держать “только fast-path + dev gate”?
2) Нужна ли поддержка генерации tool’ов не только для `internal/tools/`, но и для upstream registry (скорее нет для MVP).
3) Формат шаблонов: inline string templates vs `text/template` (MVP проще — inline + fmt).
4) Хотим ли auto-add в `searchLocalTools` всегда, или по флагу.
