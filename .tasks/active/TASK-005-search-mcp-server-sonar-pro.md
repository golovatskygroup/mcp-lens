# Task 005: MCP сервер поиска через OpenRouter Sonar Pro

## Контекст

mcp-proxy нуждается в возможности выполнять веб-поиск для получения актуальной информации. OpenRouter предоставляет доступ к модели Perplexity Sonar Pro, которая специализируется на поиске и может возвращать ответы с цитатами из источников.

## Цель

Создать Python MCP сервер, который:
1. Предоставляет инструмент `web_search` для поиска в интернете
2. Использует OpenRouter API с моделью `perplexity/sonar-pro`
3. Интегрируется с mcp-proxy как upstream сервер
4. Может быть протестирован через простой MCP клиент

## Предлагаемое решение

### 1) Python MCP сервер (`packages/search-server/`)

**Структура:**
```
packages/search-server/
├── pyproject.toml          # Зависимости: mcp, httpx
├── search_server/
│   ├── __init__.py
│   └── server.py           # MCP сервер с web_search tool
└── README.md
```

**Инструмент `web_search`:**
- Input: `query` (string) - поисковый запрос
- Output: JSON с результатами поиска и цитатами
- Использует OpenRouter API (`https://openrouter.ai/api/v1/chat/completions`)
- Модель: `perplexity/sonar-pro`
- Требует env: `OPENROUTER_API_KEY`

**Пример вызова OpenRouter:**
```python
import httpx

response = httpx.post(
    "https://openrouter.ai/api/v1/chat/completions",
    headers={
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    },
    json={
        "model": "perplexity/sonar-pro",
        "messages": [{"role": "user", "content": query}],
    },
)
```

### 2) Конфигурация для mcp-proxy

**Добавить preset в `internal/presets/presets.go`:**
```go
"search": {
    Command: "python",
    Args:    []string{"-m", "search_server"},
    Env: map[string]string{
        "OPENROUTER_API_KEY": "${OPENROUTER_API_KEY}",
    },
},
```

**Или использовать через config.yaml:**
```yaml
upstream:
  command: "python"
  args: ["-m", "search_server"]
  env:
    OPENROUTER_API_KEY: "${OPENROUTER_API_KEY}"
```

### 3) MCP клиент для тестирования (`packages/search-server/test_client.py`)

Простой клиент, который:
1. Запускает MCP сервер как subprocess
2. Отправляет `initialize` request
3. Вызывает `tools/list` для проверки доступных инструментов
4. Вызывает `tools/call` с `web_search` и тестовым запросом
5. Выводит результат

### 4) Интеграция в mcp-proxy

После создания сервера:
1. Добавить в `.env.example` документацию по `OPENROUTER_API_KEY`
2. Добавить preset "search" в registry
3. Обновить README с инструкциями

## Acceptance criteria

- [ ] Python MCP сервер создан и работает (`python -m search_server`)
- [ ] Инструмент `web_search` доступен через `tools/list`
- [ ] Поиск работает через OpenRouter Sonar Pro
- [ ] Тестовый клиент успешно вызывает поиск
- [ ] Сервер интегрирован в mcp-proxy (preset или документация)

## Тест-план

**Manual testing:**
1. Установить зависимости: `cd packages/search-server && pip install -e .`
2. Установить `OPENROUTER_API_KEY`
3. Запустить тест: `python test_client.py`
4. Проверить, что поиск возвращает результаты с цитатами

**Integration test:**
1. Запустить mcp-proxy с search upstream
2. Вызвать `query` с запросом на поиск
3. Убедиться, что результаты получены

## Зависимости

- Python 3.10+
- `mcp` (официальный Python SDK)
- `httpx` (HTTP клиент)
- `OPENROUTER_API_KEY` в environment

## Ссылки

- OpenRouter API: https://openrouter.ai/docs
- Perplexity Sonar Pro: https://openrouter.ai/perplexity/sonar-pro
- MCP Python SDK: https://github.com/modelcontextprotocol/python-sdk
