# Task 0004: Детерминированное извлечение контекста (URL/IDs) + инъекция args в plan

## Контекст

Пользователь часто даёт *URL* (Grafana dashboard link, GitHub PR URL, Jira issue URL и т.п.).
Если всё оставить на LLM, возникают ошибки:
- не извлечён UID/ID,
- не проставлен `base_url`/`org_id`,
- план получается длиннее/дороже/менее стабильным.

## Проблема

Сейчас часть “контекстной магии” реализована точечно (например, `grafana <client>` префикс и частичная URL-логика), но нет общей архитектуры и единых правил.

## Цель

Сделать распознавание сущностей (URL/ID) детерминированным и переиспользуемым для любых доменов.

## Предлагаемое решение (общее)

### 1) Общий интерфейс extractor

Ввести интерфейс вида:

- `type ContextExtractor interface { Name() string; TryExtract(input string) (map[string]any, bool) }`

и список экстракторов:
- Grafana dashboard URL → `{grafana_base_url, grafana_org_id, grafana_dashboard_uid, grafana_dashboard_url}`
- GitHub PR URL → `{github_repo, github_pr_number, github_pr_url}`
- Jira issue URL → `{jira_issue_key, jira_base_url?}`
- Confluence page URL → `{confluence_page_id, confluence_base_url?}`

### 2) Слияние в `runRouter` до планирования

- В `internal/tools/router.go`: прогоняем input через экстракторы и добавляем в `in.Context` (не перетирая уже заданный context).
- Эти значения попадают в prompt планировщика как “structured context”.

### 3) Инъекция в план после планирования

Как уже делается для `*_client`:
- пост-обработка plan шагов: если `args.base_url`/`args.org_id`/`args.uid` отсутствуют, заполнить их из контекста (но не перетирать явно заданные).

## Acceptance criteria

- При наличии URL в input план стабильно использует правильные args без “угадываний”.
- Поведение одинаково для нескольких доменов (Grafana/GitHub/Jira/Confluence).
- Никакие секреты не добавляются в контекст автоматически.

## Тест-план

Offline unit tests:
- парсинг URL → ожидаемые поля контекста;
- “args injection” не перетирает явно заданные пользователем значения;
- edge cases: URL с пунктуацией, в кавычках, внутри текста.

## Ссылки

- Grafana search (как устроены /d/<uid>/...): https://grafana.com/docs/grafana/latest/visualizations/dashboards/search-dashboards/

