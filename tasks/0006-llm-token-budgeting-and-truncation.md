# Task 0006: LLM token budgeting + защита от truncation (plan/summary) для OpenRouter

## Контекст

Даже при корректном JSON-парсинге могут случаться “обрывы” ответов из-за лимитов модели (`max_tokens`) и больших промптов (особенно при `include_answer=true`).

Это проявляется не только в Grafana: любая доменная интеграция с большими результатами может “утопить” суммаризацию.

## Проблема

- Вызов OpenRouter может не задавать `max_tokens` явно (поведение по умолчанию зависит от провайдера/модели).
- Клиент не анализирует `finish_reason` и не умеет реагировать на `length`.
- Суммаризация может получать огромные `executed_steps[].result`.

## Цель

Сделать планирование и суммаризацию устойчивыми:
- контролировать budget на выход,
- обнаруживать truncation,
- деградировать предсказуемо (artifacts + preview), а не “ломаться”.

## Предлагаемое решение (общее)

### 1) Поддержка `max_tokens` (и связанных параметров) в OpenRouterClient

Добавить конфиг (env) и отправлять в body:
- `MCP_LENS_ROUTER_MAX_TOKENS_PLAN`
- `MCP_LENS_ROUTER_MAX_TOKENS_SUMMARY`

Документация параметров: OpenRouter API.

### 2) Парсить `finish_reason`

Расширить парсинг ответа OpenRouter (choices[0].finish_reason):
- если `finish_reason == "length"`:
  - для plan: retry с ужесточённой инструкцией “return ONLY JSON” и/или меньшим catalog;
  - для summary: fallback на “short summary” + ссылки на artifacts (Task 0002).

### 3) Санитизация inputs в summarize

Перед отправкой в LLM:
- заменить большие результаты на artifact references + preview;
- ограничить количество шагов/объём `executed_steps`.

## Acceptance criteria

- При `finish_reason=length` система возвращает валидный (хотя бы короткий) ответ, без “обрывков JSON”.
- `max_tokens` контролируется конфигом и документирован.
- Суммаризация стабильна на больших payloads (через artifacts + preview).

## Тест-план

Unit tests (без реального OpenRouter):
- подставить фиктивный JSON ответа OpenRouter с `finish_reason=length` и убедиться, что включается fallback.

Integration tests (opt-in, tag `integration`):
- реальный вызов модели с большим контекстом и проверка отсутствия truncation (при наличии ключей).

## Ссылки

- OpenRouter params: https://openrouter.ai/docs/api/reference/parameters
- OpenRouter chat completions: https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request
- OpenAI structured outputs (идея строгих форматов): https://platform.openai.com/docs/guides/structured-outputs

