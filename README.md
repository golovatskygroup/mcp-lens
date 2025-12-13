# mcp-lens

## О чём проект

`mcp-lens` — MCP proxy, который запускает upstream MCP server и **уменьшает поверхность инструментов** для клиента.

Вместо того чтобы показывать десятки инструментов upstream-сервера, `mcp-lens` начинает с 3 meta-tools:
- `search_tools` — поиск доступных инструментов по ключевым словам/категориям
- `describe_tool` — схема/описание инструмента
- `execute_tool` — выполнение выбранного инструмента

По мере использования `execute_tool` proxy **авто-активирует** реальные tools для текущей сессии.

## Как установить

### 1) Скачать бинарник (recommended)

Скачай подходящий архив из GitHub Releases и распакуй бинарник `mcp-lens`.

Платформенные имена архивов (пример):
- `mcp-lens_1.0.2_darwin_arm64.tar.gz`
- `mcp-lens_1.0.2_linux_amd64.tar.gz`
- `mcp-lens_1.0.2_windows_amd64.zip`

### 2) Добавить в MCP client (stdio)

Пример для Claude Desktop / Cursor / других MCP клиентов со stdio:

```json
{
  "mcpServers": {
    "tavily-proxy": {
      "command": "/ABS/PATH/TO/mcp-lens",
      "env": {
        "MCP_LENS_UPSTREAM_COMMAND": "npx",
        "MCP_LENS_UPSTREAM_ARGS_JSON": "[\"-y\",\"<TAVILY_MCP_PACKAGE>\"]",
        "MCP_LENS_UPSTREAM_ENV_JSON": "{\"TAVILY_API_KEY\":\"${TAVILY_API_KEY}\"}",
        "TAVILY_API_KEY": "YOUR_TAVILY_KEY"
      }
    }
  }
}
```

Примечание: `<TAVILY_MCP_PACKAGE>` замени на точное имя пакета Tavily MCP (пришли его — подставлю корректное значение).

### ENV-переменные

- `MCP_LENS_PRESET` — имя готового пресета (сейчас: `github`)
- `MCP_LENS_UPSTREAM_COMMAND` — команда upstream сервера
- `MCP_LENS_UPSTREAM_ARGS_JSON` — JSON массив args
- `MCP_LENS_UPSTREAM_ENV_JSON` — JSON объект env vars (значения поддерживают `${VAR}`)

## Как помогать контрибьютить

- Issues/PR: приветствуются.
- Не добавляй реальные токены/секреты в репозиторий.
- Локальные проверки:
  - `go test ./...`
  - (wrapper) `npm -C packages/mcp-lens run build`

## Статистика

- Реализовано: GitHub (preset `github`, via `@modelcontextprotocol/server-github`)
- Дальше: GitLab, Jira, Slack, Notion, Linear
