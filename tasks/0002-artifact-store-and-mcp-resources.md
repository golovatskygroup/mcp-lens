# Task 0002: Единый artifact store + manifests + (опционально) MCP resources для больших результатов

## Контекст

Отчёты показывают повторяющуюся боль: результаты некоторых tool calls (например, Grafana dashboards) огромные, плохо читаются, а LLM-суммаризация может ломаться из-за размера контекста.

Сейчас часть клиентов/агентов сама пишет большие ответы в случайные файлы, но это:
- недетерминированно (имена файлов не говорят, что внутри);
- не переносимо между MCP-клиентами;
- усложняет суммаризацию/пост-обработку.

## Проблема

- Нет стандартного механизма “если результат большой → сохрани как artifact и верни ссылку + метаданные”.
- В `pkg/mcp` модель контента упрощена (только `{type,text}`), нет `resource`/`resource_link`.
- `executed_steps[].result` может стать гигантским и утопить `include_answer`/LLM.

## Цель

Сделать работу с большими результатами предсказуемой и одинаковой для всех доменов:
- большие результаты сохраняются как artifacts;
- в ответе возвращается компактная ссылка + manifest;
- LLM-суммаризация использует только компактные представления.

## Предлагаемое решение (общее)

### 1) Artifact store на уровне роутера (не на уровне отдельных tools)

Добавить компонент (например, `internal/artifacts/`) и интегрировать в `executePlan(...)`:
- измерять размер сериализованного `st.Result`;
- если больше порога (например, `MCP_LENS_ARTIFACT_INLINE_MAX_BYTES`):
  - записать JSON/текст в файл в `MCP_LENS_ARTIFACT_DIR` (default: `os.TempDir()`),
  - посчитать `sha256`,
  - заменить `ExecutedStep.Result` на компактный объект:
    - `artifact_path`, `sha256`, `bytes`, `mime`, `preview` (первые N байт/полей).
- добавить общий `artifacts`/`manifest` в `router.RouterResult` (сводка по всем артефактам).

### 2) Нейминг и стабильность

Детерминированное имя файла:
- `<tool>-<primary-id>-<timestamp>-<shortid>.json`
- `primary-id` извлекается из args: `uid`, `repo+number`, `issue`, etc.
- если `primary-id` отсутствует → `query`.

### 3) (Опционально, но желательно) MCP resources

Расширить протоколную часть сервера:
- реализовать `resources/list` и `resources/read` для чтения артефактов через MCP resources;
- расширить `pkg/mcp.ContentBlock` под `resource`/`resource_link` (в соответствии со spec).

Это позволит клиентам получать артефакты “протокольно”, а не через локальный путь.

## Acceptance criteria

- При больших результатах `executed_steps[].result` содержит ссылку на artifact, а не полный payload.
- Ответ `query` включает `manifest` со списком созданных artifacts (имя, размер, sha256, tool, args-сводка).
- В `include_answer=true` суммаризация не падает/не деградирует из-за размера результатов (за счёт preview + ссылок).

## Тест-план

Offline unit tests:
- “большой result” → создаётся файл + sha256 + manifest заполняется.
- “маленький result” → inline, без artifacts.

Если реализуем MCP resources:
- `resources/read` возвращает содержимое сохранённого artifact.

## Ссылки

- MCP resources spec: https://modelcontextprotocol.io/specification/2025-06-18/server/resources
- MCP tools spec (content types): https://modelcontextprotocol.io/specification/2025-06-18/server/tools

