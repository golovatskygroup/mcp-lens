# Task 0005: Общий HTTP cache слой (ETag/If-None-Match + TTL) для локальных tools

## Контекст

Повторные вызовы `grafana_get_dashboard*`, `jira_get_issue`, `confluence_get_page`, GitHub PR reads и т.п. часто повторяются в одной сессии.
Без кэша это:
- лишняя нагрузка,
- риск rate limits,
- более медленный UX.

## Проблема

Сейчас каждый домен реализует HTTP вызовы самостоятельно, без общего кэша и без conditional requests.

## Цель

Добавить общий кэш-слой для HTTP GET/идемпотентных запросов:
- безопасный (не смешивает ответы разных auth/clients),
- конфигурируемый,
- прозрачный для доменных tool handlers.

## Предлагаемое решение (общее)

### 1) Общий пакет, например `internal/httpcache/`

Функции:
- in-memory cache (TTL + max entries)
- ключ: `method + url + org_id + auth_fingerprint + extra_headers_fingerprint`
- хранить: `status`, `headers (ETag)`, `body`, `stored_at`

### 2) Conditional requests

Для GET:
- если есть сохранённый `ETag`, отправлять `If-None-Match`;
- при `304 Not Modified` возвращать cached body.

### 3) Конфиг через env

- `MCP_LENS_HTTP_CACHE_ENABLED=1`
- `MCP_LENS_HTTP_CACHE_TTL_SECONDS=60`
- `MCP_LENS_HTTP_CACHE_MAX_ENTRIES=512`

### 4) Интеграция

Переиспользовать в доменных клиентах (Grafana/Jira/Confluence/GitHub), начиная с GET-эндпоинтов.

## Acceptance criteria

- При повторном GET одного и того же URL в рамках TTL делается либо 0 запросов (TTL), либо 304 revalidate (ETag).
- Кэш не пересекает разные auth (разные токены → разные cache keys).
- Можно выключить кэш полностью env-переменной.

## Тест-план

Offline unit tests с `httptest`:
- первый запрос возвращает `ETag`, второй должен отправить `If-None-Match`;
- сервер отвечает 304, клиент возвращает прошлое тело.

## Ссылки

- MDN `Cache-Control`: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Cache-Control
- MDN `ETag`: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/ETag
- MDN `If-None-Match`: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/If-None-Match

