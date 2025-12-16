# План изменений для mcp-lens (mcp-proxy)

Цель: убрать любые вызовы **external MCP tools** (upstream MCP server), оставить только внутренние (локальные) возможности и добавить флоу “скрипт → результат → регистрация → повторное использование как тулза”.

---

## 0) Что есть сейчас (как устроен репо)

- `internal/server/server.go`: MCP JSON-RPC сервер. Сейчас:
  - запускает upstream MCP процесс через `internal/proxy` (child process),
  - грузит upstream схемы в `internal/registry`,
  - наружу через `tools/list` показывает **только** `query`,
  - `query/router` внутри умеют планировать/валидировать/исполнять шаги (локальные и потенциально upstream).
- `internal/tools/*`: набор локальных read-only утилит (GitHub/Jira/Confluence/Grafana + meta tools).
- `internal/router/*`: планирование (OpenRouter), строгая валидация плана + read-only policy.
- `internal/proxy/*`, `internal/presets/*`, `config*.yaml`: инфраструктура для upstream MCP сервера.

Требование “убрать external mcp tools вызовы совсем” означает: удалить/отключить весь путь `upstream` (процесс, схемы, вызовы).

---

## 1) Цели и ограничения

### Цели

1. Полностью убрать upstream MCP:
   - не запускать дочерний MCP процесс,
   - не хранить и не показывать upstream tools,
   - не исполнять upstream tool calls нигде (ни напрямую, ни через router).
2. Ввести “внешние API адаптеры” как первоклассную сущность:
   - их можно **изучать** (список адаптеров, операции, схемы, примеры вызова),
   - их можно **вызывать** через безопасный слой.
3. Добавить флоу “скриптинг → закрепление в тулзу”:
   - клиент генерирует код/скрипт на основе каталога адаптеров,
   - сервер исполняет скрипт и возвращает ответ,
   - при успехе клиент регистрирует новую тулзу (описание, вход/выход, случаи применения),
   - при следующем подключении эта тулза уже доступна и вызывается напрямую.
4. Добавить end-to-end тесты, которые проверяют весь путь детерминированно.

### Ограничения/принципы

- Базовый `go test ./...` должен быть **offline** и детерминированным (без реальных внешних API).
- Внешние интеграции допускаются только как opt-in тесты через build tags (`integration`, `e2e`) + `t.Skip` без env.
- Read-only политика должна сохраняться для вызовов внешних API (адаптеры должны быть “read-only по умолчанию”).

---

## 2) Целевой пользовательский флоу (без upstream MCP)

1. Клиент вызывает `adapters_list` → получает список адаптеров и операций (+ схемы/описания).
2. Клиент генерирует “клиентский код” (или скрипт) на основе этого каталога.
3. Клиент вызывает `sandbox_compile` (или аналог) → сервер компилирует/валидирует код “клиента”, возвращает `client_id`.
4. Клиент вызывает `sandbox_run` с `client_id` + скриптом → сервер исполняет, возвращает результат.
5. Если результат ок, клиент вызывает `tool_register`:
   - имя тулзы,
   - случаи применения/теги,
   - что требует (input schema),
   - что возвращает (output schema или контракт результата),
   - исполняемый скрипт/программа.
6. На следующем вызове/старте сервера:
   - новая тулза появляется в `tools/list`,
   - клиент просто вызывает её напрямую (без повторного написания скрипта).

---

## 3) Предлагаемая архитектура

### 3.1 Adapter Registry (каталог адаптеров)

Добавить `internal/adapters/`:

- `type Adapter interface { Name() string; Ops() []Operation }`
- `type Operation struct { Name, Description string; InputSchema, OutputSchema json.RawMessage; Call(ctx, args) (any, error) }`
- `type Registry struct { ListAdapters(); Describe(adapter, op); Call(adapter, op, args) }`

Интеграция с текущими локальными тулзами (`internal/tools/*`):

- Вариант A (быстрее): “адаптер” = префикс (`jira_*`, `confluence_*`, `grafana_*`, `github_*`), `Call` делегирует в существующий `tools.Handler.Handle`.
- Вариант B (чище): выделить реальные адаптеры как отдельные Go-клиенты и использовать их напрямую, а тулзы оставить тонким слоем.

На первом этапе рекомендуется A (минимум рефакторинга), затем при необходимости перейти на B.

### 3.2 Sandbox / Script execution

Нужен “исполнитель кода” (скриптов), который:

- не даёт доступ к файловой системе/процессам,
- ограничен по времени выполнения,
- умеет вызывать **только** зарегистрированные адаптеры/операции.

Рекомендуемый вариант: `go.starlark.net` (Starlark) как безопасный embeddable язык.

Слой `internal/sandbox/`:

- компиляция/проверка скриптов,
- поддержка `client_id` (in-memory сессия: сохранение набора функций/объекта “клиента” между вызовами),
- builtins:
  - `adapters()` → возвращает каталог (для codegen внутри скрипта),
  - `call(adapter, op, args)` → вызывает адаптер (args = dict),
  - `json_encode/ json_decode` (опционально),
  - `print` в лог (опционально).

### 3.3 Dynamic Tool Store (постоянные “прикрученные” тулзы)

Добавить `internal/dynamictools/` (или `internal/toolstore/`):

- Хранилище на диске (JSON файл/директория) + атомарная запись (tmp + rename).
- Модель:
  - `name`, `description`,
  - `use_cases`/`tags`/`keywords`,
  - `input_schema`, `output_schema`,
  - `program` (скрипт Starlark или иной формат),
  - `created_at`, `version`, `hash`.
- Валидация при регистрации:
  - имя не конфликтует с системными,
  - имя не “mutating” (create/update/delete/…),
  - schema валидный JSON Schema (compile),
  - скрипт компилируется,
  - (опционально) dry-run/verification: выполнить с `example_args` и убедиться что результат сериализуем в JSON.

Интеграция:

- `tools.Handler.BuiltinTools()` должен добавлять dynamic tools в `[]mcp.Tool` (с их схемами).
- `tools.Handler.Handle(...)`:
  - если имя не найдено в статическом switch, проверять dynamic registry,
  - исполнять dynamic tool через `internal/sandbox` (скрипт) и возвращать результат.
- `internal/router/policy.go`:
  - добавить поддержку dynamic tools (например, `AllowDynamic map[string]struct{}` или `AllowLocalPrefixes`),
  - или автоматически разрешать tools из toolstore, если они read-only и прошли валидацию.

### 3.4 MCP surface: какие тулзы экспонировать наружу

После удаления upstream, предлагается экспонировать наружу “спец-тулзы” + dynamic tools:

- `adapters_list`
- `adapters_describe`
- `sandbox_compile` (создать `client_id` из кода)
- `sandbox_run` (исполнить скрипт с `client_id`)
- `tool_register` (зарегистрировать тулзу)
- `tool_list` / `tool_describe` (для админки/дебага)
- + все зарегистрированные dynamic tools

`query` можно:

- либо оставить как удобный entrypoint (LLM-планировщик), но без upstream,
- либо оставить только спец-тулзы (если система переезжает в “programmatic-first” режим).

Рекомендуется сохранить `query`, но добавить env-флаг:
- `MCP_LENS_EXPOSE_ALL_TOOLS=1` → показывать полный список тулз (спец + dynamic + адаптерные тулзы при необходимости),
- по умолчанию можно оставить текущий UX (только `query`) и дать новый режим для клиентов, которым нужен прямой доступ.

---

## 4) План работ (итерации)

### Итерация 1 — удалить upstream MCP полностью

- Удалить/вырезать путь `internal/proxy`, `internal/presets`, `cmd/proxy` upstream-конфиг, `internal/registry` в upstream-части.
- Обновить `internal/server/server.go`:
  - убрать `proxy.Start/ListTools/CallTool`,
  - перестать грузить upstream tools,
  - `tools/list` формировать только из внутренних тулз.
- Обновить README + `.env.example` (убрать `MCP_LENS_PRESET`, `MCP_LENS_UPSTREAM_*`).
- Обновить тесты, которые завязаны на registry/upstream (если есть).

### Итерация 2 — каталог адаптеров

- Ввести `internal/adapters` (registry + описания).
- Добавить тулзы:
  - `adapters_list` (json/text),
  - `adapters_describe` (детальная схема + пример args).
- Тесты (unit): каталог корректно группирует/описывает операции.

### Итерация 3 — sandbox compile/run

- Добавить `internal/sandbox` на базе Starlark:
  - `Compile(code) -> client_id`,
  - `Run(client_id, script, input) -> result`.
- Добавить тулзы:
  - `sandbox_compile`,
  - `sandbox_run`.
- Тесты (unit): таймауты, запреты, корректный вызов mock-адаптера.

### Итерация 4 — регистрация dynamic tool + доступность в tools/list

- Добавить `internal/dynamictools` (file store) + загрузка на старте.
- Добавить тулзу `tool_register` (+ `tool_list`/`tool_describe`).
- Встроить dynamic tools в:
  - `tools.Handler.BuiltinTools()` (схемы),
  - `tools.Handler.Handle()` (исполнение),
  - `router.DefaultPolicy()`/`Policy` (разрешение dynamic tools).
- Тесты (unit): валидация имен/схем/скриптов, атомарная запись, загрузка.

### Итерация 5 — e2e тест “полный флоу”

Детерминированный e2e (без сети):

1. Поднять `tools.Handler` + `server.Server` с temp-dir toolstore.
2. Зарегистрировать mock-адаптер (например `echo`/`math`) в `internal/adapters` только для теста.
3. `adapters_list` → убедиться что mock есть.
4. `sandbox_compile` → получить `client_id`.
5. `sandbox_run` → получить успешный ответ.
6. `tool_register` → зарегистрировать новую тулзу (например `echo_uppercase`).
7. `tools/list` → убедиться что новая тулза появилась.
8. `tools/call` этой тулзы → убедиться что результат совпадает.
9. Создать новый инстанс сервера (эмуляция рестарта) → тулза всё ещё доступна.

Опционально: e2e под build tag `integration` с реальными Jira/Confluence/Grafana (только при наличии env).

---

## 5) Конфиги / env, которые понадобятся

Предложенные переменные:

- `MCP_LENS_TOOLSTORE_DIR` или `MCP_LENS_TOOLSTORE_PATH` (где хранить dynamic tools)
- `MCP_LENS_SANDBOX_TIMEOUT_MS` (лимит выполнения скрипта)
- `MCP_LENS_EXPOSE_ALL_TOOLS` (режим экспонирования тулз в `tools/list`)

---

## 6) Вопросы для согласования (нужно решить до реализации)

1. Скриптовый язык: Starlark (рекомендуется) vs JS (goja) vs “JSON-plan DSL”.
2. Хотим ли мы по умолчанию продолжать показывать только `query`, или перейти на “полный список тулз”?
3. Нужен ли префикс для dynamic tools (например `custom_*`) для упрощения policy и избежания коллизий?
4. Persistence: достаточно хранить dynamic tools в файле на диске (локально), или нужен удалённый store (не в рамках этого репо)?

