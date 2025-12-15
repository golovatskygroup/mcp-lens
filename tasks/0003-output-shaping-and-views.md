# Task 0003: Универсальное output shaping (include/exclude fields, views) для JSON-heavy tools

## Контекст

Для многих API (Grafana dashboards, GitHub diff/files, Jira issues, Confluence pages) “полный JSON” слишком шумный.
Пользователи хотят:
- быстро получить *суть* (summary/view),
- или выбрать *подмножество* полей без ручного парсинга.

## Проблема

Сейчас есть два экстремума:
- вернуть “всё” (огромно),
- или делать отдельный tool на каждую “упрощённую” форму (разрастание surface area).

## Цель

Дать единый механизм управления формой ответа, применимый к любому tool (local и upstream), при сохранении backwards compatibility.

## Предлагаемое решение (общее)

### 1) Расширить аргументы `query` (backwards compatible)

Добавить опциональный блок, например:

- `output`:
  - `view`: `"full" | "summary" | "metadata" | ...`
  - `include_fields`: `[]string` (white-list, пути)
  - `exclude_fields`: `[]string` (black-list, пути)
  - `max_items`: int (для списков)
  - `max_depth`: int (для nested объектов)
  - `redact`: `[]string` (пути для принудительной редактуры)

Важно: не делать “произвольный JSONPath”, достаточно безопасных путей в стиле JSON Pointer/`a.b[0].c` с ограниченным парсером.

### 2) Применение shaping на уровне `executePlan` результата

После выполнения шага:
- нормализовать `any` → `map/[]any`;
- применить shaping (view/projection) к `ExecutedStep.Result`;
- сохранять исходник в artifact store (Task 0002) при необходимости.

### 3) Views как тонкий слой над shaping

Для часто используемых сущностей задать “view presets”:
- `summary`: оставляет title/name + ключевые поля + “targets/queries”
- `metadata`: только meta/идентификаторы
- `errors_only`: только ошибки/статусы

Views должны быть доменно-агностичными по умолчанию (общие поля), а доменные расширения — через таблицу маппинга `tool_name -> view preset`.

## Acceptance criteria

- `query` умеет возвращать компактные ответы без доп. custom tools.
- `include_fields` уменьшает размер ответа минимум в 5–10 раз на JSON-heavy примерах.
- Default поведение не меняется (если `output` не задан).

## Тест-план

Offline unit tests:
- projection engine: include/exclude для map/arrays, ошибки на невалидные пути.
- view presets: для нескольких типов результатов (map, list).

## Ссылки

- Grafana dashboard API (payload большой по определению): https://grafana.com/docs/grafana/latest/developer-resources/api-reference/http-api/dashboard/
- Pattern “sparse fieldsets” (концепт): https://atendesigngroup.com/articles/json-api-getting-just-data-you-need-sparse-fieldsets

