# План: Динамическая генерация тулз для MCP сервера

## Обзор

Модификация MCP proxy сервера для поддержки динамической генерации и регистрации инструментов на основе внешних API адаптеров.

## Ключевые изменения

1. **Убрать external MCP tools вызовы** - оставить только внутренние специальные тулзы
2. **Система внешних API адаптеров** с предопределёнными адаптерами (REST, GraphQL, OpenAPI)
3. **Динамическая генерация кода** через LLM на основе описания задачи
4. **Регистрация новых тулз** после успешного выполнения
5. **Persistence** для сохранения тулз между перезапусками

---

## Архитектура

### Структура файлов

```
mcp-proxy/
├── internal/
│   ├── adapter/
│   │   ├── adapter.go              # Интерфейсы адаптеров
│   │   ├── registry.go             # Реестр адаптеров
│   │   └── implementations/
│   │       ├── rest_adapter.go     # REST адаптер
│   │       └── openapi_parser.go   # OpenAPI spec парсер
│   ├── executor/
│   │   ├── executor.go             # Главный executor (роутинг по языку)
│   │   ├── native/
│   │   │   ├── go_executor.go      # Yaegi для Go
│   │   │   ├── js_executor.go      # goja для JavaScript
│   │   │   └── python_executor.go  # Python exec
│   │   ├── docker/
│   │   │   ├── manager.go          # Docker runtime manager
│   │   │   ├── container.go        # Container lifecycle
│   │   │   └── images.go           # Образы для языков
│   │   ├── sandbox.go              # Sandbox конфигурация
│   │   └── validator.go            # Валидация кода
│   ├── tool/
│   │   ├── registry.go             # Реестр инструментов
│   │   ├── builtin.go              # Встроенные тулзы (6 штук)
│   │   ├── dynamic.go              # Динамические тулзы
│   │   └── discover.go             # discover_api тулза (Perplexity)
│   ├── discovery/
│   │   ├── perplexity.go           # Perplexity MCP client
│   │   ├── strategies.go           # Стратегии поиска API
│   │   ├── openapi_extractor.go    # Извлечение OpenAPI из результатов
│   │   └── chain.go                # Цепочки запросов
│   ├── persistence/
│   │   ├── db.go                   # SQLite подключение
│   │   ├── tool_store.go           # Хранение тулз + версии + язык
│   │   └── migrations.go           # Миграции БД
│   ├── cache/
│   │   └── cache.go                # In-memory кеш тулз
│   └── config/
│       └── config.go               # Конфигурация
├── migrations/
│   └── 001_initial_schema.sql
├── tests/
│   └── e2e/
│       ├── workflow_test.go        # Full workflow E2E
│       ├── multilang_test.go       # Multi-language E2E
│       ├── docker_test.go          # Docker runtime E2E
│       ├── persistence_test.go     # Persistence E2E
│       └── errors_test.go          # Error scenarios E2E
└── config/
    └── config.yaml
```

---

## Workflow диаграмма

```
┌─────────────────┐
│   LLM Agent     │  (Claude, GPT, etc.)
│   MCP Client    │
└────────┬────────┘
         │
         ├─→ [1] list_adapters ─────────→ получить схемы API адаптеров
         │
         │   ╔══════════════════════════════════════════════╗
         │   ║  LLM агент САМ генерирует код на основе      ║
         │   ║  полученных схем (не нужна отдельная тулза)  ║
         │   ╚══════════════════════════════════════════════╝
         │
         ├─→ [2] execute_code ──────────→ выполнить сгенерированный код
         │                                в Yaegi sandbox
         │
         │   ╔══════════════════════════════════════════════╗
         │   ║  Если результат устраивает клиента...        ║
         │   ╚══════════════════════════════════════════════╝
         │
         ├─→ [3] register_tool ─────────→ сохранить код как новую тулзу
         │                                (name, description, code, schema)
         │
         │                          ┌──────────────────┐
         │                          │  MCP Server      │
         │                          │  - ToolRegistry  │
         │                          │  - Persistence   │
         │                          │  - Executor      │
         │                          └──────────────────┘
         │
         └─→ [4] my_registered_tool ───→ вызов напрямую (без генерации!)
```

### Пример сценария

1. **Клиент**: `list_adapters` → получает схему GitHub API
2. **LLM**: Видит схему, генерирует код для получения PR
3. **Клиент**: `execute_code(code)` → получает результат
4. **Клиент**: Результат хороший → `register_tool("get_pr", code)`
5. **Следующий раз**: `get_pr(owner, repo, pr_number)` → напрямую!

---

## Встроенные тулзы (6 штук)

> **Важно**: Тулза `generate_code` убрана, т.к. LLM агент сам генерирует код на основе схем адаптеров.
> **Новое**: Добавлена тулза `discover_api` для автоматического поиска API документации через Perplexity.

### 1. `list_adapters`
Возвращает список доступных API адаптеров и их полные схемы.

```json
{
  "name": "list_adapters",
  "description": "List all available API adapters with their schemas",
  "inputSchema": {
    "type": "object",
    "properties": {
      "filter": {"type": "string", "description": "Filter by type"}
    }
  }
}
```

### 2. `execute_code`
Выполняет код на любом языке. Нативные runtime (Go, JS, Python) - напрямую, остальные - через Docker.

```json
{
  "name": "execute_code",
  "description": "Execute code in any language. Native: go, javascript, python. Others: via Docker container.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "language": {
        "type": "string",
        "enum": ["go", "javascript", "python", "ruby", "rust", "java", "php", "bash"],
        "description": "Programming language"
      },
      "code": {
        "type": "string",
        "description": "Code to execute"
      },
      "input": {
        "type": "object",
        "description": "Input parameters (available as JSON in code)"
      },
      "timeout": {
        "type": "string",
        "default": "30s"
      }
    },
    "required": ["language", "code"]
  }
}
```

**Возможные ответы:**

```json
// Успешное выполнение
{
  "status": "success",
  "result": { ... },
  "execution_time_ms": 150,
  "language": "python"
}

// Контейнер запускается (для не-нативных языков)
{
  "status": "container_starting",
  "language": "rust",
  "container_id": "abc123",
  "message": "Container is starting. Retry in 5 seconds.",
  "retry_after_seconds": 5
}

// Ошибка
{
  "status": "error",
  "error": "syntax error on line 5",
  "language": "python"
}
```

### 3. `start_runtime`
Заранее поднимает Docker контейнер для языка (чтобы не ждать при execute_code).

```json
{
  "name": "start_runtime",
  "description": "Pre-start a Docker container for a language runtime. Use before execute_code for non-native languages.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "language": {
        "type": "string",
        "enum": ["ruby", "rust", "java", "php", "bash", "typescript"],
        "description": "Language to start runtime for"
      }
    },
    "required": ["language"]
  }
}
```

**Ответ:**
```json
{
  "status": "ready",
  "language": "rust",
  "container_id": "abc123",
  "message": "Runtime ready. You can now call execute_code with language='rust'"
}
```

### 4. `register_tool`
Регистрирует успешно выполненный код как новую тулзу.

```json
{
  "name": "register_tool",
  "description": "Register executed code as a new reusable tool",
  "inputSchema": {
    "type": "object",
    "properties": {
      "name": {"type": "string", "description": "Tool name (snake_case)"},
      "description": {"type": "string"},
      "language": {"type": "string"},
      "code": {"type": "string"},
      "input_schema": {"type": "object"}
    },
    "required": ["name", "description", "language", "code", "input_schema"]
  }
}
```

### 5. `rollback_tool`
Откатывает тулзу на предыдущую версию.

### 6. `discover_api` (NEW - Perplexity Integration)
Автоматический поиск API документации и OpenAPI спецификаций через Perplexity.

```json
{
  "name": "discover_api",
  "description": "Discover API documentation and OpenAPI specs for any service using Perplexity AI",
  "inputSchema": {
    "type": "object",
    "properties": {
      "service_name": {
        "type": "string",
        "description": "Name of the service (e.g., 'Stripe', 'GitHub', 'Slack')"
      },
      "search_strategy": {
        "type": "string",
        "enum": ["openapi_first", "full_discovery", "endpoints_only"],
        "default": "openapi_first",
        "description": "Discovery strategy"
      },
      "focus_areas": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Specific areas to focus on (e.g., 'authentication', 'rate limits')"
      }
    },
    "required": ["service_name"]
  }
}
```

**Стратегии поиска:**

| Стратегия | Описание | Инструменты Perplexity |
|-----------|----------|------------------------|
| `openapi_first` | Сначала ищет OpenAPI/Swagger spec | `search` → если не найден → `reason` |
| `full_discovery` | Полное исследование API | `deep_research` с focus_areas |
| `endpoints_only` | Только список endpoints | `search` + `reason` цепочка |

**Ответ:**

```json
{
  "status": "success",
  "openapi_spec_url": "https://github.com/stripe/openapi/blob/master/openapi/spec3.json",
  "openapi_available": true,
  "api_overview": {
    "base_url": "https://api.stripe.com",
    "auth_methods": ["bearer", "basic"],
    "categories": ["payments", "customers", "subscriptions"],
    "total_endpoints": 300
  },
  "sources": [
    "https://docs.stripe.com/api",
    "https://github.com/stripe/openapi"
  ],
  "recommendation": "Use openapi-generator with spec3.json for full client generation"
}
```

---

## Perplexity API Discovery Strategy

### Исследование (проведено 2025-12-16)

Протестированы три инструмента Perplexity MCP для поиска API документации:

| Инструмент | Лучший сценарий | Оценка для codegen |
|------------|-----------------|-------------------|
| `search` | Поиск OpenAPI spec | ⭐⭐⭐⭐⭐ (5/5) |
| `reason` | Анализ структуры API, цепочки | ⭐⭐⭐⭐½ (4.5/5) |
| `deep_research` | Полная документация | ⭐⭐⭐⭐ (8/10) |

### Оптимальная цепочка запросов

```
┌─────────────────────────────────────────────────────────────────┐
│  Шаг 1: search("{ServiceName} OpenAPI specification swagger")   │
│         ↓                                                       │
│  Найден OpenAPI? ──→ YES ──→ Скачать spec, использовать        │
│         │                    openapi-generator                  │
│         NO                                                      │
│         ↓                                                       │
│  Шаг 2: reason("How to get {ServiceName} API schema for SDK")  │
│         ↓                                                       │
│  Шаг 3: search("List all {ServiceName} API categories")        │
│         ↓                                                       │
│  Шаг 4: Для каждой категории:                                  │
│         reason("{ServiceName} {Category} API endpoints detail") │
│         ↓                                                       │
│  Шаг 5: Агрегировать результаты → Сгенерировать клиент         │
└─────────────────────────────────────────────────────────────────┘
```

### Результаты тестирования

**Stripe API** (search):
- ✅ Найден `github.com/stripe/openapi/spec3.json`
- Оценка: 5/5 для codegen

**GitHub API** (reason + chain):
- ✅ Найден `github.com/github/rest-api-description`
- 17 категорий API с детальной документацией
- Оценка: 4.5/5 для codegen

**Slack API** (deep_research):
- ✅ 30+ методов с описаниями
- ✅ 6 типов токенов аутентификации
- ✅ Rate limits по тирам
- ⚠️ Нет официального OpenAPI spec
- Оценка: 8/10 для codegen

**Notion API** (chain of 5 queries):
- ✅ Обнаружено отсутствие официального OpenAPI
- ✅ Найдены альтернативы (Levo.ai, DocuWriter.ai)
- ✅ Production-ready workflow для scraping
- Оценка: 9/10 для полноты информации

### Ключевые выводы

1. **Лучший первый запрос**: `"{ServiceName} OpenAPI specification swagger.json"`
2. **5 запросов достаточно** для полного покрытия API без OpenAPI
3. **Распределение инструментов:**
   - `search` → факты (что? где?)
   - `reason` → анализ (как? trade-offs?)
   - `deep_research` → полная картина с 60+ источниками
4. **Автоматизация возможна** через последовательные вызовы

### Интеграция в workflow

```
┌─────────────────┐
│   LLM Agent     │
└────────┬────────┘
         │
         ├─→ [0] discover_api ─────────→ Найти OpenAPI spec или структуру API
         │                               через Perplexity
         │
         ├─→ [1] list_adapters ────────→ Показать найденный API как адаптер
         │                               (автоматически зарегистрирован)
         │
         ├─→ [2] execute_code ─────────→ LLM генерирует код на основе schema
         │
         └─→ [3] register_tool ────────→ Сохранить как переиспользуемую тулзу
```

---

## Интерфейсы

### Adapter Interface

```go
type Adapter interface {
    GetID() string
    GetType() AdapterType
    GetSchema(ctx context.Context) (*Schema, error)
    Execute(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error)
    Validate() error
}

type AdapterType string

const (
    AdapterTypeREST    AdapterType = "rest"
    AdapterTypeGraphQL AdapterType = "graphql"
    AdapterTypeOpenAPI AdapterType = "openapi"
)
```

### Tool Definition

```go
type ToolDefinition struct {
    ID            string                 `json:"id"`
    Name          string                 `json:"name"`
    Description   string                 `json:"description"`
    Code          string                 `json:"code"`
    InputSchema   map[string]interface{} `json:"input_schema"`
    Version       int                    `json:"version"`
    Status        string                 `json:"status"`
    CreatedAt     time.Time              `json:"created_at"`
    UpdatedAt     time.Time              `json:"updated_at"`
}
```

### Multi-Language Code Executor

```go
type Language string

const (
    LangGo         Language = "go"
    LangJavaScript Language = "javascript"
    LangPython     Language = "python"
    LangRuby       Language = "ruby"
    LangRust       Language = "rust"
    LangJava       Language = "java"
    LangPHP        Language = "php"
    LangBash       Language = "bash"
    LangTypeScript Language = "typescript"
)

// Нативные runtime (без Docker)
var NativeRuntimes = map[Language]bool{
    LangGo:         true,  // Yaegi
    LangJavaScript: true,  // goja
    LangPython:     true,  // go-python или exec
}

type CodeExecutor interface {
    Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)
    StartRuntime(ctx context.Context, language Language) (*RuntimeStatus, error)
    StopRuntime(ctx context.Context, language Language) error
    ListRuntimes(ctx context.Context) ([]RuntimeStatus, error)
}

type ExecuteRequest struct {
    Language Language               `json:"language"`
    Code     string                 `json:"code"`
    Input    map[string]interface{} `json:"input,omitempty"`
    Timeout  time.Duration          `json:"timeout"`
}

type ExecuteResponse struct {
    Status          string      `json:"status"` // success, error, container_starting
    Result          interface{} `json:"result,omitempty"`
    Error           string      `json:"error,omitempty"`
    Language        Language    `json:"language"`
    ExecutionTimeMs int64       `json:"execution_time_ms"`
    ContainerID     string      `json:"container_id,omitempty"`
    RetryAfterSec   int         `json:"retry_after_seconds,omitempty"`
}

type RuntimeStatus struct {
    Language    Language `json:"language"`
    Status      string   `json:"status"` // ready, starting, stopped
    ContainerID string   `json:"container_id,omitempty"`
    StartedAt   string   `json:"started_at,omitempty"`
}
```

### Runtime Manager (Docker)

```go
type RuntimeManager interface {
    // Запустить контейнер для языка
    Start(ctx context.Context, lang Language) (*RuntimeStatus, error)

    // Выполнить код в контейнере
    Execute(ctx context.Context, containerID string, code string, input interface{}) (interface{}, error)

    // Остановить контейнер
    Stop(ctx context.Context, containerID string) error

    // Получить статус
    Status(ctx context.Context, lang Language) (*RuntimeStatus, error)
}

// Docker образы для каждого языка
var LanguageImages = map[Language]string{
    LangRuby:       "ruby:3.3-alpine",
    LangRust:       "rust:1.75-alpine",
    LangJava:       "eclipse-temurin:21-alpine",
    LangPHP:        "php:8.3-cli-alpine",
    LangBash:       "alpine:3.19",
    LangTypeScript: "node:20-alpine",  // с ts-node
}
```

> **Примечание**: Нативные языки (Go, JS, Python) исполняются без Docker для скорости.

### API Discovery Client (Perplexity)

```go
type DiscoveryStrategy string

const (
    StrategyOpenAPIFirst   DiscoveryStrategy = "openapi_first"
    StrategyFullDiscovery  DiscoveryStrategy = "full_discovery"
    StrategyEndpointsOnly  DiscoveryStrategy = "endpoints_only"
)

type DiscoveryClient interface {
    // Основной метод - поиск API документации
    DiscoverAPI(ctx context.Context, req DiscoveryRequest) (*DiscoveryResult, error)

    // Низкоуровневые методы для цепочек
    Search(ctx context.Context, query string) (*PerplexityResponse, error)
    Reason(ctx context.Context, query string) (*PerplexityResponse, error)
    DeepResearch(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error)
}

type DiscoveryRequest struct {
    ServiceName    string            `json:"service_name"`
    Strategy       DiscoveryStrategy `json:"search_strategy"`
    FocusAreas     []string          `json:"focus_areas,omitempty"`
    MaxQueries     int               `json:"max_queries,omitempty"` // default: 5
}

type DiscoveryResult struct {
    Status           string      `json:"status"` // success, partial, not_found
    OpenAPISpecURL   string      `json:"openapi_spec_url,omitempty"`
    OpenAPIAvailable bool        `json:"openapi_available"`
    APIOverview      *APIOverview `json:"api_overview,omitempty"`
    Sources          []string    `json:"sources"`
    Recommendation   string      `json:"recommendation"`
    QueriesUsed      int         `json:"queries_used"`
}

type APIOverview struct {
    BaseURL        string   `json:"base_url"`
    AuthMethods    []string `json:"auth_methods"`
    Categories     []string `json:"categories"`
    TotalEndpoints int      `json:"total_endpoints"`
    RateLimits     string   `json:"rate_limits,omitempty"`
}

type PerplexityResponse struct {
    Content  string   `json:"content"`
    Sources  []string `json:"sources"`
    Model    string   `json:"model"`
}
```

---

## Безопасность исполнения кода

Используем **Yaegi** (Go интерпретатор) вместо exec.Command:

```go
type SafeCodeExecutor struct {
    allowedPackages map[string]bool
    timeout         time.Duration
}

var allowedPackages = map[string]bool{
    "encoding/json": true,
    "fmt":           true,
    "strings":       true,
    "strconv":       true,
    "math":          true,
    "time":          true,
    "regexp":        true,
    "sort":          true,
}

var forbiddenPatterns = []string{
    "exec.Command",
    "os.Setenv",
    "ioutil.WriteFile",
    "os.Mkdir",
    "os.Remove",
    "syscall",
    "unsafe",
    "cgo",
}
```

---

## Persistence (SQLite)

### Схема БД

```sql
CREATE TABLE tools (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    code TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tool_versions (
    id TEXT PRIMARY KEY,
    tool_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    code TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(tool_id) REFERENCES tools(id),
    UNIQUE(tool_id, version)
);

CREATE TABLE tool_executions (
    id TEXT PRIMARY KEY,
    tool_id TEXT NOT NULL,
    status TEXT NOT NULL,
    input_params TEXT,
    result TEXT,
    error_message TEXT,
    execution_time_ms INTEGER,
    executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## E2E тесты

### Test 1: Full Workflow
```
1. list_adapters → получить список адаптеров
2. generate_code → сгенерировать код для задачи
3. execute_code → выполнить код
4. register_tool → зарегистрировать тулзу
5. call tool → вызвать зарегистрированную тулзу напрямую
```

### Test 2: Persistence
```
1. Зарегистрировать тулзу
2. Перезапустить сервер
3. Проверить что тулза сохранилась
```

### Test 3: Error Scenarios
```
- Невалидный адаптер
- Синтаксическая ошибка в коде
- Timeout при выполнении
- Отсутствующие required поля
```

### Test 4: Versioning
```
1. Создать тулзу v1
2. Обновить тулзу → v2
3. Откатить на v1
4. Проверить что код v1
```

---

## Конфигурация

```yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 15s
  write_timeout: 15s

database:
  path: ./data/mcp.db
  migrations_path: ./migrations

cache:
  enabled: true
  ttl: 5m
  max_size: 1000

execution:
  timeout: 5s
  max_memory_mb: 256
  allowed_packages:
    - encoding/json
    - fmt
    - strings
    - strconv
    - math
    - time
    - regexp
    - sort
    - net/http  # для REST вызовов
    - bytes
    - io

adapters:
  - id: github_api
    type: rest
    base_url: https://api.github.com
    auth:
      type: bearer
      token_env: GITHUB_TOKEN
  - id: jira_api
    type: rest
    base_url: ${JIRA_BASE_URL}
    auth:
      type: basic
      user_env: JIRA_USER
      token_env: JIRA_TOKEN

# Perplexity API Discovery
discovery:
  enabled: true
  perplexity:
    # MCP server endpoint (если используется через MCP)
    mcp_server: "perplexity"
    # Или прямой API (если используется напрямую)
    api_key_env: PERPLEXITY_API_KEY
    base_url: https://api.perplexity.ai
  cache:
    enabled: true
    ttl: 24h  # Кешировать результаты discovery на 24 часа
    max_entries: 100
  defaults:
    strategy: openapi_first
    max_queries: 5
    timeout: 60s
```

---

## Этапы реализации

### Этап 1: Базовая инфраструктура
- [ ] Создать структуру директорий
- [ ] Реализовать интерфейс Adapter
- [ ] Реализовать REST адаптер с OpenAPI парсингом
- [ ] Настроить SQLite + миграции (с полем `language`)

### Этап 2: Native Code Executors
- [ ] Реализовать Go executor (Yaegi)
- [ ] Реализовать JavaScript executor (goja)
- [ ] Реализовать Python executor
- [ ] Добавить валидацию кода для каждого языка
- [ ] Добавить timeout и memory limits

### Этап 3: Docker Runtime Manager
- [ ] Реализовать Docker client wrapper
- [ ] Реализовать container lifecycle (start/stop/exec)
- [ ] Добавить пул контейнеров (warm containers)
- [ ] Реализовать `start_runtime` тулзу
- [ ] Добавить retry логику при `container_starting`

### Этап 4: Tool Registry & Persistence
- [ ] Реализовать ToolStore (SQLite) с полем language
- [ ] Реализовать ToolCache (in-memory)
- [ ] Добавить версионирование тулз
- [ ] Добавить rollback функционал

### Этап 5: MCP Integration
- [ ] Реализовать `list_adapters` тулзу
- [ ] Реализовать `execute_code` тулзу (мультиязычный)
- [ ] Реализовать `start_runtime` тулзу
- [ ] Реализовать `register_tool` тулзу
- [ ] Реализовать `rollback_tool` тулзу
- [ ] Добавить динамическую регистрацию тулз в runtime

### Этап 5.5: Perplexity API Discovery (NEW)
- [ ] Реализовать Perplexity MCP client (`internal/discovery/perplexity.go`)
- [ ] Реализовать стратегии поиска:
  - [ ] `openapi_first` - поиск OpenAPI spec через `search`
  - [ ] `full_discovery` - полное исследование через `deep_research`
  - [ ] `endpoints_only` - цепочка `search` + `reason`
- [ ] Реализовать OpenAPI URL extractor из текстовых результатов
- [ ] Реализовать цепочку запросов с контекстом
- [ ] Реализовать `discover_api` тулзу
- [ ] Добавить кеширование результатов discovery
- [ ] Интегрировать с adapter registry (автоматическая регистрация найденных API)

### Этап 6: Testing
- [ ] Unit тесты для каждого executor
- [ ] Integration тесты Docker runtime
- [ ] E2E тесты (full workflow)
- [ ] E2E тесты (multi-language)
- [ ] E2E тесты (discover_api → register adapter → generate code)
- [ ] E2E тесты (container_starting retry)
- [ ] E2E тесты (persistence после restart)
- [ ] E2E тесты (error scenarios)

### Этап 7: CI/CD
- [ ] Makefile
- [ ] GitHub Actions workflow (с Docker-in-Docker)
- [ ] Dockerfile
- [ ] Health checks

---

## Зависимости

```go
require (
    // Database
    github.com/mattn/go-sqlite3 v1.14.22
    github.com/golang-migrate/migrate/v4 v4.17.0

    // Native executors
    github.com/traefik/yaegi v0.16.1       // Go interpreter
    github.com/dop251/goja v0.0.0-20240220    // JavaScript VM

    // Docker
    github.com/docker/docker v25.0.0
    github.com/docker/go-connections v0.5.0

    // Utils
    github.com/google/uuid v1.6.0

    // Testing
    github.com/stretchr/testify v1.9.0
    github.com/testcontainers/testcontainers-go v0.31.0
)
```

---

## Риски и митигации

| Риск | Митигация |
|------|-----------|
| LLM генерирует небезопасный код | Строгая валидация + whitelist + sandbox |
| Timeout при выполнении | Context с таймаутом + container limits |
| Docker контейнер не запускается | Retry логика + `container_starting` статус |
| Долгий cold start контейнера | `start_runtime` для pre-warming + пул контейнеров |
| Конфликты имён тулз | Unique constraint в БД |
| Потеря данных | Версионирование + rollback |
| Высокая нагрузка | Кеширование + rate limiting |
| Docker daemon недоступен | Fallback на нативные языки + graceful degradation |
| Perplexity API недоступен | Кеширование результатов + fallback на cached specs |
| OpenAPI spec не найден | Fallback на endpoints_only стратегию через reason |
| Результаты Perplexity неточные | Валидация URL + проверка доступности spec |

---

## Источники

1. https://www.glukhov.org/ru/post/2025/07/mcp-server-in-go/
2. https://habr.com/ru/companies/cloud_ru/articles/935390/
3. https://github.com/traefik/yaegi
4. https://github.com/mark3labs/mcp-go
5. Perplexity MCP API Discovery Research (2025-12-16) - внутреннее исследование
6. https://github.com/stripe/openapi - пример OpenAPI spec
7. https://github.com/github/rest-api-description - GitHub REST API OpenAPI
