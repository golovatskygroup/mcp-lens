# Задача: Проверка работы discover_mcp_servers.py

## Описание
Проверить и валидировать работу скрипта `discover_mcp_servers.py`, который использует search-server для поиска информации о MCP серверах и генерации конфигураций пресетов для mcp-proxy.

## Контекст
- **Документация:** `packages/search-server/README.md`, `TASK-005-search-mcp-server-sonar-pro.md`
- **Целевые файлы:** `packages/search-server/discover_mcp_servers.py`
- **Зависимости:** TASK-005 (search-server должен работать)

## TODO

### Phase 1: Проверка окружения
- [x] **1.1** Убедиться, что search-server установлен и работает
  - Установить зависимости: `cd packages/search-server && uv pip install -r requirements.txt --system`
  - Проверить наличие `OPENROUTER_API_KEY` в environment
  - Запустить `python test_client.py` для проверки базовой работы
  - Файл: `packages/search-server/test_client.py`
  - **Исправлено:** Добавлен `search_server/__main__.py` для работы `python -m search_server`

- [x] **1.2** Проверить зависимости discover_mcp_servers.py
  - `mcp` SDK (v1.25.0)
  - `python-dotenv` (v1.2.1)
  - Файл: `packages/search-server/requirements.txt`

### Phase 2: Запуск и валидация discover_mcp_servers.py
- [x] **2.1** Запустить скрипт и проверить выполнение
  - Команда: `cd packages/search-server && python discover_mcp_servers.py`
  - Проверить, что скрипт без ошибок подключается к search-server
  - Проверить, что поиск выполняется для всех известных серверов (KNOWN_MCP_SERVERS)
  - Файл: `packages/search-server/discover_mcp_servers.py`
  - **Улучшено:** Добавлены флаги `--no-search` и `--timeout`

- [x] **2.2** Проверить генерацию выходных данных
  - Убедиться, что создается файл `discovered_presets.json`
  - Проверить формат JSON (валидный, содержит поля `command`, `args`)
  - Проверить вывод Go-кода в консоль (формат для presets.go)
  - Файл: `packages/search-server/discovered_presets.json`

### Phase 3: Проверка качества результатов
- [x] **3.1** Валидировать сгенерированные пресеты
  - Проверить, что все 12 известных серверов обработаны ✓
  - Убедиться, что npm-пакеты имеют корректные имена (@modelcontextprotocol/server-*) ✓
  - Проверить, что args содержат `-y` для npx (no prompt) ✓

- [x] **3.2** Сравнить с существующими пресетами
  - Сравнить результаты с `internal/presets/presets.go`
  - **Результат:** Все 12 discovered серверов уже есть в presets.go
  - **Дополнительные в presets.go:** `search`, `sequential-thinking` (не из @modelcontextprotocol)
  - Файл: `internal/presets/presets.go`

### Phase 4: Улучшения (выполнено)
- [x] **4.1** Добавить обработку ошибок
  - Timeout для поисковых запросов (флаг `--timeout`, по умолчанию 30с)
  - Валидация результатов поиска
  - Режим `--no-search` для генерации без API

- [ ] **4.2** Расширить список известных серверов (опционально)
  - Добавить новые серверы из официального списка MCP
  - Добавить community-серверы

## Статус
- **Создана:** 2025-12-28
- **Обновлена:** 2025-12-28
- **Статус:** Завершена ✓

## Заметки
- Скрипт требует `OPENROUTER_API_KEY` для работы (использует search-server с Perplexity Sonar Pro)
- Текущая реализация использует фиксированные npm-пакеты (@modelcontextprotocol/server-*), поиск служит для валидации
- Вывод в консоль - готовый Go-код для копирования в presets.go
- JSON-файл создается для программной интеграции

## Критерии приемки
- [x] Скрипт успешно запускается без ошибок
- [x] Файл `discovered_presets.json` создается с валидным JSON
- [x] Go-код в консоли имеет корректный синтаксис
- [x] Все 12 серверов из KNOWN_MCP_SERVERS обработаны
- [x] Результаты можно использовать для обновления presets.go

## Команды для проверки

```bash
# 1. Установка зависимостей
cd packages/search-server
pip install -e .

# 2. Проверка API ключа
echo $OPENROUTER_API_KEY

# 3. Проверка базового search-server
python test_client.py "test query"

# 4. Запуск discover_mcp_servers.py
python discover_mcp_servers.py

# 5. Проверка результата
cat discovered_presets.json | python -m json.tool
```
