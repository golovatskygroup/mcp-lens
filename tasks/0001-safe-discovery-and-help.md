# Task 0001: Safe discovery (search/describe) и встроенный help без LLM

## Контекст

В отчётах об использовании (см. `experience-mcp-lens.md`) встречается проблема: запросы вида “какие Grafana tools доступны?” приводят к плану с `search_tools`, но выполнение блокируется политикой read-only.

Это системная UX-проблема: пользователь не может “узнать возможности” прокси без внешней документации/угадывания.

## Проблема

- `search_tools` и `describe_tool` — read-only операции, но сейчас не разрешены policy для роутера, поэтому `query` не может отвечать на discovery-вопросы.
- Даже при наличии каталога в prompt модель всё равно может выбрать `search_tools` как правильный инструмент и получить отказ.

## Цель

Сделать tool discovery надёжным и безопасным:
- discovery всегда работает в режиме read-only;
- discovery не требует LLM (детерминированный “fast-path”);
- при этом `query` остаётся единственным публичным tool.

## Предлагаемое решение (общее)

### 1) Разрешить discovery в policy

Добавить `search_tools` и `describe_tool` в `internal/router/policy.go` → `DefaultPolicy().AllowLocal`.

Основание: в MCP инструменты могут иметь `readOnlyHint`, а discovery — это “метаданные”, не побочные эффекты.

### 2) Детерминированный “help/discovery fast-path” внутри `query`

В `internal/tools/router.go` (в `runRouter`) добавить до вызова OpenRouter:
- распознавание интентов `help`, `list tools`, `search tools`, `describe tool <name>` (минимальный парсер на строках/regex);
- выполнение соответствующего локального обработчика напрямую:
  - `handleSearch(...)` для `search_tools`
  - `handleDescribe(...)` для `describe_tool`

Плюсы:
- не зависит от LLM, не тратит токены;
- не создаёт план, значит не упирается в `max_steps`;
- одинаково полезно для любых доменов (Grafana/Jira/Confluence/GitHub/upstream).

### 3) Защита от утечек

При выдаче схем:
- не включать реальные секреты (их и так нет в schema), но проверить что нигде не “протекают” env/headers;
- опционально (флагом) уметь “обрезать” слишком большие schema/catlog (см. Task 0002/0003).

## Acceptance criteria

- `query` на запрос типа “search for tools related to grafana” возвращает список tools и не требует `OPENROUTER_API_KEY`.
- `query` на “describe_tool grafana_get_dashboard” возвращает schema и описание.
- Политика read-only по-прежнему блокирует mutating tools (`create/update/delete/...`), но discovery инструменты разрешены.

## Тест-план

Offline unit tests (без сети):
- тест на policy: `search_tools`/`describe_tool` разрешены как local.
- тест на `Handler.Handle("query", ...)`: при входе “help/tools/search tools” выполняется fast-path и не происходит попытки создать OpenRouterClient (т.е. работает без env).

## Ссылки

- MCP tools (concepts, hints): https://modelcontextprotocol.io/legacy/concepts/tools
- MCP tools spec (server/tools): https://modelcontextprotocol.io/specification/2025-06-18/server/tools

