# Задача: Добавить MCP Server Presets

## Описание
Добавить популярные MCP серверы как presets в mcp-proxy для упрощения конфигурации. Создать Python клиент для обнаружения серверов через web_search.

## Контекст
- **Документация:** README.md, .env.example
- **Целевые файлы:** internal/presets/presets.go, internal/presets/presets_test.go, packages/search-server/discover_mcp_servers.py, .env.example
- **Зависимости:** нет

## TODO

### Phase 1: Python MCP Discovery Client
- [x] **1.1** Создать `packages/search-server/discover_mcp_servers.py`
  - MCP клиент для подключения к search-server
  - Поиск информации о популярных MCP серверах
  - Генерация Go кода для presets
  - Файл: `packages/search-server/discover_mcp_servers.py`

### Phase 2: Go Presets Implementation
- [x] **2.1** Добавить новые presets в `NewRegistry()`
  - filesystem, sqlite, postgres, puppeteer
  - brave-search, memory, fetch, git
  - slack, gitlab, google-maps, sequential-thinking
  - Файл: `internal/presets/presets.go`
- [x] **2.2** Обновить `ListAvailable()` с новыми presets
  - Файл: `internal/presets/presets.go`
- [x] **2.3** Добавить тесты для новых presets
  - TestRegistryGetFilesystemPreset
  - TestRegistryGetSqlitePreset
  - TestRegistryGetPostgresPreset
  - TestRegistryGetBraveSearchPreset
  - TestRegistryGetSlackPreset
  - TestRegistryGetGitlabPreset
  - TestRegistryGetGoogleMapsPreset
  - TestRegistryGetSequentialThinkingPreset
  - TestAllPresetsExistInRegistry
  - Файл: `internal/presets/presets_test.go`

### Phase 3: Documentation
- [x] **3.1** Обновить `.env.example` с новыми env переменными
  - POSTGRES_CONNECTION_STRING
  - BRAVE_API_KEY
  - SLACK_BOT_TOKEN, SLACK_TEAM_ID
  - GITLAB_TOKEN, GITLAB_API_URL
  - GOOGLE_MAPS_API_KEY
  - Файл: `.env.example`

## Статус
- **Создана:** 2025-12-28
- **Обновлена:** 2025-12-28
- **Статус:** Завершена

## Заметки
Добавлены 12 новых presets для официальных MCP серверов от Anthropic:
- Без env переменных: filesystem, sqlite, puppeteer, memory, fetch, git, sequential-thinking
- С env переменными: postgres, brave-search, slack, gitlab, google-maps

Все presets используют npx для запуска npm пакетов из @modelcontextprotocol/* namespace.

## Критерии приемки
- [x] Все тесты проходят: `go test ./internal/presets/...`
- [x] Проект собирается: `go build ./...`
- [x] Документация обновлена в .env.example
- [x] Python discovery script создан
