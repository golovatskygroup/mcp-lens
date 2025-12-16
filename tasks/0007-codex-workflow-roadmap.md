# mcp-lens × Codex: план улучшений (execution-first)

Цель: сделать так, чтобы Codex мог работать “как тимлид с несколькими сессиями”, а `mcp-lens` давал ему быстрый, детерминированный и безопасный (read-only) доступ к контексту (PR/CI, Jira, Confluence, Grafana) и к сохранению “плана/заметок” между сессиями.

Ниже — **план работ** + **Perplexity-ресёрч** для каждого пункта (ссылки/факты для решений на этапе проектирования).

---

## 0) Принципы проектирования (инварианты)

- Публичный интерфейс MCP: **один tool** `query` (как сейчас).
- Политика по умолчанию: read-only относительно внешних систем (GitHub/Jira/Confluence/Grafana).
- Большие результаты: предпочитать **artifact** + короткий summary, чтобы не “забивать контекст”.
- Детерминизм: URL/ID extraction и “threading” client alias делать в коде, не в LLM.
- Offline/Dry-run: по возможности давать полезный результат без LLM (как fast-path сейчас).

---

## 1) Executor mode: `query.steps[]` (bring-your-own-plan)

### Зачем
Из статьи: “plan before coding”, “можно сохранять план в файл”, “долгая автономная работа”. Для Codex важно уметь **самому** строить план, а `mcp-lens` — только валидировать и исполнять его.

### Изменения (концепт)
- Расширить `query`/`router` schema (в `internal/tools/meta.go`) параметром:
  - `steps`: массив `{name, source, args, reason?}` — эквивалент `router.ModelPlan.Steps`.
  - `final_answer_needed` (опционально) или вычислять из `include_answer`.
- Поведение:
  - Если `steps` присутствует → **не вызывать OpenRouter**, а:
    1) собрать catalog,
    2) валидировать steps against policy + schema,
    3) исполнить steps,
    4) (опционально) summarization (если включен OpenRouter и `include_answer=true`) или deterministic fallback.
- Добавить `mode: "auto"|"planner"|"executor"` (опционально):
  - `executor` требует `steps`,
  - `planner` запрещает `steps` и делает только планирование (LLM),
  - `auto` — текущая логика.

### Файлы
- `internal/tools/meta.go` (schema)
- `internal/tools/router.go` (парсинг input, ветвление planner/executor)
- `internal/router/router.go` (реюз `ValidatePlan` / возможно новая `ValidateStepsOnly`)
- `internal/router/types.go` (если нужно расширить структуры)

### Acceptance criteria
- `query` работает без `OPENROUTER_API_KEY` в режиме `executor`, если все шаги — локальные tools.
- Ошибка валидации возвращает понятное сообщение (какой step, что не так).

### Perplexity-ресёрч (для решений)
- MCP tools/list & tools/call и роль inputSchema: https://modelcontextprotocol.io/specification/2024-11-05/server/tools
- MCP resources и “resource_link/embedded resources” (как связать artifacts): https://modelcontextprotocol.info/specification/2024-11-05/server/resources/

---

## 2) Строгая валидация args по JSON Schema (не только “object”)

### Зачем
Сейчас `ValidatePlan` проверяет лишь “args must be JSON object”. Для Codex executor mode нужен “компилятор плана”: быстрые, точные ошибки по полям/типам.

### Дизайн-варианты
1) Подключить библиотеку JSON Schema (Draft 7/2019/2020) и валидировать `args` по `inputSchema`.
2) Минимальный валидатор без зависимости: типы + required + enum + ranges (частично), но это быстро разрастется и хуже DX.

### Рекомендация
Выбрать 1 библиотеку с:
- поддержкой хотя бы draft-07 (многие схемы именно такие),
- понятными ошибками (path + message),
- приемлемым весом/зависимостями.

### Файлы
- `internal/router/router.go` (`ValidatePlan`: добавить schema validation)
- возможно `internal/registry` (доступ к inputSchema по toolName)

### Acceptance criteria
- При неверных args возвращается ошибка вида: `args schema validation failed for <tool>: <path>: <message>`.

### Perplexity-ресёрч
- `kaptinlin/jsonschema` (Draft 2020-12, хорошие ошибки): https://github.com/kaptinlin/jsonschema
- `santhosh-tekuri/jsonschema` (широко используемая lib, много draft): https://github.com/santhosh-tekuri/jsonschema

---

## 3) GitHub CI feedback loop: Actions runs/jobs/logs (read-only)

### Зачем
Из статьи: “CI failed → paste logs → ask Codex to propose fixes”. Нужно, чтобы Codex мог сам вытянуть логи/ошибки из GitHub Actions через `query`.

### Изменения (локальные tools)
- `github_list_workflow_runs` (по repo + опционально по branch/PR SHA)
- `github_list_workflow_jobs` (по run_id)
- `github_download_job_logs` (по job_id) → сохранять в artifact (текстовый .log)
  - важно: GitHub API возвращает 302 redirect на временный URL (1 мин).

### Файлы
- `internal/tools/github_*.go` (новые handlers)
- `internal/tools/meta.go` (tool registration + searchLocalTools)
- `internal/router/policy.go` (AllowLocal)

### Acceptance criteria
- По PR можно получить список workflow runs и скачать логи **конкретного failed job** в artifact.

### Perplexity-ресёрч
- Workflow jobs + “Download job logs” (302 redirect, expires ~1 minute): https://docs.github.com/en/rest/actions/workflow-jobs
- Workflow runs endpoints: https://docs.github.com/en/rest/actions/workflow-runs
- Важно учитывать нюансы прав/скоупов (часто нужен `repo` scope для private): https://github.com/orgs/community/discussions/24742

---

## 4) GitHub review context: reviews + review comments + issue comments (read-only)

### Зачем
Для качественного ревью/доработок Codex’у нужен контекст обсуждения: что уже попросили исправить, где замечания в diff.

### Изменения (локальные tools)
- `github_list_pull_request_reviews`
- `github_list_pull_request_review_comments`
- `github_list_issue_comments` (у PR как у issue)

### Файлы
- `internal/tools/github_*.go`
- `internal/tools/meta.go`
- `internal/router/policy.go`

### Acceptance criteria
- `query` “summarize review feedback for PR …” возвращает список ключевых замечаний и их привязку к файлам/линиям (где возможно).

### Perplexity-ресёрч
- Pull request review comments API: https://docs.github.com/en/rest/pulls/comments
- Pull request reviews API: https://docs.github.com/en/rest/pulls/reviews
- Issue comments (PR timeline) API: https://docs.github.com/en/rest/issues/comments

---

## 5) Jira “issue bundle”: issue + comments + changelog (+ links/parents)

### Зачем
Для превращения тикета в план реализации Codex’у нужен: описание, acceptance criteria, последние комментарии, изменения статуса/поля, связи.

### Изменения
- Новый tool `jira_get_issue_bundle`:
  - вход: `issue`, опционально `fields`, `comments_limit`, `include_changelog`, `include_links`
  - выход: компактная структура + `has_next` если комментариев много + artifacts при больших объемах.
- Опционально: `jira_get_issue_changelog` отдельным tool (если bundle слишком тяжелый).

### Риски
- `expand=changelog` может сделать ответы **очень большими**, а “история” в Jira не всегда полная/очевидная (нужно документировать ограничения).

### Acceptance criteria
- `query` “jira PROJ-123 summarize requirements and recent changes” дает стабильный, “сжимаемый” результат (с artifacts при необходимости).

### Perplexity-ресёрч
- Правильное использование `expand=changelog` как отдельного query параметра (не внутри JQL): https://community.atlassian.com/forums/Jira-questions/How-to-get-issues-from-Jira-projects-using-expand-changelog/qaq-p/1476328
- Jira Cloud REST v3 Issues group (права/scopes, issue details): https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/
- Прагматика про changelog и его “дырки”: https://improvingflow.atlassian.net/wiki/spaces/technicalexcellence.com/blog/2024/04/09/jira-issue-history.html

---

## 6) Confluence “context pack”: поиск → дерево → вложения

### Зачем
Из статьи: “контекст — всё”. Для реального engineering Codex’у нужны дизайн-доки/ранбуки (часто это дерево страниц + вложения).

### Изменения
- `confluence_get_page_children` (с пагинацией; Cloud v2 + fallback v1/DC)
- `confluence_list_page_attachments`
- (опционально) `confluence_download_attachment` → сохранить файл в artifact store (read-only внешне, но пишет локально как и прочие artifacts)
- `confluence_get_page_ancestors` (для понимания раздела)

### Acceptance criteria
- По URL страницы: найти 3–10 наиболее релевантных child pages и вложения, вернуть компактный индекс + ссылки/ids.

### Perplexity-ресёрч
- Confluence Data Center: `GET /rest/api/content/{id}?expand=body.storage` и children: https://support.atlassian.com/confluence/kb/how-to-get-page-content-or-child-list-via-rest-api/
- Confluence Cloud REST v2 children (пагинация через `Link` header): https://developer.atlassian.com/cloud/confluence/rest/v2/api-group-children/
- Cloud v1 “child by type” (полезно для attachments): https://developers.qodex.ai/atlassian-confluence/atlassian-confluence-cloud/api-content-id-child/get-content-children-by-type

---

## 7) Grafana alert inventory (read-only) + “dashboard → alerts” связка

### Зачем
Для надежности “как на проде” Codex’у нужно уметь:
- смотреть не только dashboards, но и alerts/rules,
- понимать покрытие алертами (что мониторится, чего не хватает).

### Изменения
- `grafana_list_alerts` (legacy `GET /api/alerts` если доступно)
- `grafana_list_alert_rules` (unified alerting / provisioning API; версионность!)
- `grafana_get_alert_rule` (по uid/id если применимо)
- `grafana_prepare_dashboard_observability_bundle`:
  - dashboard summary
  - datasources
  - (если возможно) релевантные alerts/rules по dashboard uid/folder

### Риски
Grafana APIs различаются по версиям (legacy vs unified alerting), часть эндпоинтов “полудокументирована” и ведет себя по-разному.

### Acceptance criteria
- Запрос “review dashboard … suggest alerts gaps” возвращает список существующих алертов и предложения новых.

### Perplexity-ресёрч
- Unified alerting API обсуждение и `/api/ruler/...` paths: https://community.grafana.com/t/grafana-new-unified-alerting-api/70156
- Общая дока про alert rules (термины и модели): https://grafana.com/docs/grafana/latest/alerting/fundamentals/alert-rules/
- API `GET /api/alerts` (legacy; пример в AWS Managed Grafana docs): https://docs.aws.amazon.com/grafana/latest/userguide/Grafana-API-Alerting.html

---

## 8) Artifacts как “память”: save/append/search (для длинных задач)

### Зачем
Из статьи: “сохраняли план в файл при упоре в context window”. Сейчас artifacts создаются автоматически для больших результатов, но нет удобного “нотебука”.

### Изменения (локальные tools)
- `artifact_save_text` (input: `name?`, `text`, `mime?`) → artifact:// URI + path
- `artifact_append_text` (по `artifact_id`/`artifact_uri`) → новая версия/новый artifact (иммутабельнее)
- `artifact_list` / `artifact_search` (по названию, sha, времени)

### Важно про policy
Названия tools должны оставаться “немутирующими” для внешних систем. Это локальная запись в artifact dir; но текущая политика блокирует по substring `write`, поэтому избегать `*_write_*`.

### Acceptance criteria
- Можно: “save this plan as artifact named …” и потом: “load artifact … and continue”.

### Perplexity-ресёрч
- MCP resources/read/list как стандартный механизм отдачи файлов: https://modelcontextprotocol.info/specification/2024-11-05/server/resources/

---

## 9) Быстрее без потери качества: параллельное выполнение независимых шагов

### Зачем
Из статьи: “массово параллельные сессии”. Даже внутри одного `query` часто можно параллелить: несколько dashboard summaries, несколько Jira issues, PR details + checks.

### Изменения (вариант)
- В `executePlan(...)` использовать `errgroup.WithContext` и лимит `SetLimit(n)` для запуска step’ов:
  - либо “авто”: параллелить только явно независимые типы (например, несколько `grafana_get_dashboard_summary` подряд),
  - либо явная аннотация в step: `{"parallel_group":"g1"}` (тогда можно параллелить в рамках группы).

### Acceptance criteria
- Запрос “compare 3 dashboards …” выполняется заметно быстрее, но порядок выдачи результатов стабилен (сортируем по исходному step index).

### Perplexity-ресёрч
- `errgroup` + `WithContext` + `SetLimit`: https://leapcell.io/blog/managing-concurrent-tasks-in-go-with-errgroup
- Rate limiting (например, `x/time/rate`) как защита от API лимитов: https://www.alexedwards.net/blog/how-to-rate-limit-http-requests

---

## 10) Hardening: метод-aware policy + endpoint allowlists (безопасность при расширении)

### Зачем
Сейчас блокировка мутаций основана на имени (`create/update/delete/write/...`). Это хорошо как первый барьер, но с ростом числа tools стоит усилить гарантии.

### Изменения
- Для HTTP-based tools добавить явное поле “HTTP method” в коде выполнения (и разрешать только GET/HEAD по умолчанию).
- Для “опасных” API (например, Grafana alert provisioning) завести allowlist путей.
- Добавить redaction (секреты/токены) на уровне output shaping.

### Acceptance criteria
- Даже если появится новый tool, который случайно умеет POST, он не выполнится без явного разрешения в policy.

### Perplexity-ресёрч
- ETag/If-None-Match как часть безопасного кэша GET (в том числе для снижения нагрузки/лимитов): https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/If-None-Match
- Практическая схема ETag caching: http://rednafi.com/misc/etag_and_http_caching/

---

## 11) Документация “как работать с агентом” (Codex onboarding)

### Зачем
Из статьи: “Treat it like a new senior engineer”, “AGENTS.md повсюду”, “план → код”. Нужно закрепить рабочий процесс в репо.

### Изменения
- Добавить в `README.md` раздел “Codex workflow”:
  - шаблоны промптов для `dry_run`, `executor`, “bundle” инструментов,
  - рекомендации по `output` shaping.
- Добавить пример `~/.codex/AGENTS.md` (пути к репам, типовые команды).
- Добавить в `tasks/` короткий playbook: “Explore → Plan → Execute (steps) → Save artifact”.

### Acceptance criteria
- Новый пользователь за 5 минут понимает: как получить план, как выполнить steps, как сохранить/продолжить.

### Perplexity-ресёрч
- Практика “инструкционных файлов” для агентного кодинга (CLAUDE.md/AGENTS.md): https://www.anthropic.com/engineering/claude-code-best-practices
- Обзор формата/иерархии AGENTS.md для Codex: https://aruniyer.github.io/blog/agents-md-instruction-files.html

---

## Предлагаемая последовательность внедрения (2–3 итерации)

### Итерация A (макс. эффект для Codex)
1) Executor mode (`query.steps[]`) + понятные ошибки
2) JSON Schema validation для args
3) GitHub Actions logs (CI feedback loop)

### Итерация B (контекст из внутренних систем)
4) Jira issue bundle
5) Confluence context pack
6) Grafana alert inventory

### Итерация C (качество/скорость/операционка)
7) Artifact notebook (save/append/search)
8) Параллельное выполнение шагов + rate limiting
9) Усиление policy (method-aware / allowlists)
10) Документация/кукбук

