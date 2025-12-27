# Задача: Расширение пресетов MCP серверов и CLI wizard для настройки

## Описание
Добавить все найденные MCP серверы в presets registry и реализовать интерактивный CLI wizard для пошаговой настройки каждого сервера с вводом необходимых данных (API ключи, токены, пути и т.д.).

## Контекст
- **Документация:** `README.md`, `docs/`, `packages/search-server/README.md`
- **Целевые файлы:** `internal/presets/presets.go`, `cmd/proxy/main.go`, новый `internal/cli/wizard.go`
- **Зависимости:** TASK-007 (discover_mcp_servers validation)

## TODO

### Phase 1: Расширение presets registry
- [x] **1.1** Добавить новые reference серверы из modelcontextprotocol/servers
  - `everything` - тестовый сервер (prompts, resources, tools)
  - `time` - время и timezone конвертация
  - Файл: `internal/presets/presets.go`

- [x] **1.2** Добавить популярные community серверы
  - `tavily` - Tavily AI search (требует TAVILY_API_KEY)
  - `redis` - Redis database operations
  - `mongodb` - MongoDB database operations
  - `notion` - Notion workspace integration
  - `linear` - Linear project management
  - `sentry` - Sentry error tracking
  - `aws` - AWS services integration
  - `kubernetes` - Kubernetes cluster management
  - Файл: `internal/presets/presets.go`

- [x] **1.3** Добавить metadata к пресетам
  - Описание сервера (description)
  - Требуемые env vars (required_env)
  - Опциональные env vars (optional_env)
  - Категория (category: reference/database/search/productivity/devtools)
  - Файл: `internal/presets/presets.go`

### Phase 2: CLI wizard для настройки серверов
- [x] **2.1** Создать базовую структуру wizard
  - Новый пакет `internal/cli/`
  - Интерактивный ввод с подсказками
  - Валидация введенных данных
  - Файл: `internal/cli/wizard.go`

- [x] **2.2** Реализовать команду `mcp-lens setup`
  - Показать список доступных пресетов с описаниями
  - Выбор пресета из списка (fzf-style или numbered list)
  - Пошаговый ввод required env vars
  - Опциональный ввод optional env vars
  - Файл: `cmd/proxy/main.go`

- [x] **2.3** Генерация конфигурации
  - Вывод готового YAML для config.yaml
  - Вывод команд export для .env или shell
  - Опционально: автоматическая запись в файл
  - Файл: `internal/cli/wizard.go`

### Phase 3: Улучшения UX
- [x] **3.1** Группировка пресетов по категориям
  - Reference: everything, memory, fetch, filesystem, git, time, sequential-thinking
  - Database: sqlite, postgres, redis, mongodb
  - Search: brave-search, tavily, search (local)
  - Productivity: slack, notion, linear, github, gitlab
  - DevTools: puppeteer, sentry, aws, kubernetes
  - Файл: `internal/presets/presets.go`

- [x] **3.2** Проверка доступности сервера
  - Проверить что npx/uvx доступен
  - Проверить что пакет существует (npm view / pip show)
  - Опционально: тест запуска сервера
  - Файл: `internal/cli/wizard.go`

- [x] **3.3** Документация пресетов
  - Ссылки на документацию каждого сервера (DocsURL в PresetMeta)
  - Примеры использования (в README.md)
  - Файл: `README.md`

### Phase 4: Интеграция и тестирование
- [x] **4.1** Обновить main.go
  - Добавить флаг `--setup` или subcommand `setup`
  - Добавить флаг `--list-presets` для вывода списка
  - Добавить флаг `--check <preset>` для проверки готовности
  - Файл: `cmd/proxy/main.go`

- [x] **4.2** Написать тесты
  - Unit тесты для нового preset metadata
  - Тесты для wizard logic (без реального stdin)
  - Файл: `internal/presets/presets_test.go`, `internal/cli/wizard_test.go`

- [x] **4.3** Обновить документацию
  - README.md: добавить секцию про setup wizard
  - .env.example: добавить все новые env vars
  - Файл: `README.md`, `.env.example`

## Статус
- **Создана:** 2025-12-28
- **Обновлена:** 2025-12-28
- **Статус:** Завершена ✅

## Заметки

### Новые серверы для добавления (из поиска):

**Reference (официальные):**
- `@modelcontextprotocol/server-everything` - тестовый
- `@modelcontextprotocol/server-time` - время/timezone

**Community (популярные):**
- Tavily Search - `mcp-server-tavily` или аналог
- Redis - `@modelcontextprotocol/server-redis` или community
- MongoDB - community servers
- Notion - `@notionhq/mcp-server` или community
- Linear - community
- Sentry - community
- AWS - community
- Kubernetes - community

### Пример metadata структуры:

```go
type PresetMeta struct {
    Name        string
    Description string
    Category    string // reference, database, search, productivity, devtools
    Config      proxy.Config
    RequiredEnv []EnvVar
    OptionalEnv []EnvVar
    DocsURL     string
}

type EnvVar struct {
    Name        string
    Description string
    Example     string
    Secret      bool // маскировать ввод
}
```

### Пример wizard flow:

```
$ mcp-lens setup

🔧 MCP Lens Server Setup Wizard

Available preset categories:
  1. Reference (official MCP servers)
  2. Database (postgres, sqlite, redis, mongodb)
  3. Search (brave, tavily)
  4. Productivity (github, gitlab, slack, notion)
  5. DevTools (puppeteer, sentry, kubernetes)

Select category [1-5]: 4

Available presets in Productivity:
  1. github    - GitHub API integration
  2. gitlab    - GitLab API integration
  3. slack     - Slack workspace integration
  4. notion    - Notion workspace integration

Select preset [1-4]: 1

Configuring: github
GitHub API integration for repositories, issues, and pull requests.

Required configuration:
  GITHUB_TOKEN: Your GitHub Personal Access Token
  → Enter value (hidden): ********

Optional configuration:
  GITHUB_API_URL: Custom GitHub API URL (for GitHub Enterprise)
  → Enter value or press Enter to skip: 

✅ Configuration complete!

# Add to your shell profile or .env:
export GITHUB_TOKEN="ghp_xxxx..."

# Or use config.yaml:
upstream:
  preset: github
  env:
    GITHUB_PERSONAL_ACCESS_TOKEN: "ghp_xxxx..."

# Run with:
mcp-lens --config config.yaml
```

## Критерии приемки
- [x] Все найденные серверы добавлены в presets registry
- [x] CLI wizard работает интерактивно для всех пресетов
- [x] Пароли/токены маскируются при вводе
- [x] Генерируется валидный YAML и shell export команды
- [x] Тесты проходят
- [x] Документация обновлена

## Команды для проверки

```bash
# Сборка
go build -o mcp-lens ./cmd/proxy

# Список пресетов
./mcp-lens --list-presets

# Интерактивная настройка
./mcp-lens setup

# Тесты
go test ./internal/presets/... ./internal/cli/...
```
