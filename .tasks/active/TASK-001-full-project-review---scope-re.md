# Задача: Full project review + scope reduction to smart MCP router (context-saving)

## Описание
Провести ревью всего репозитория, зафиксировать недостатки и затем привести проект к заявленной цели: только «умный роутер» для MCP, который экономит контекст (минимизирует токены в промпте/контексте за счет отбора инструментов и детерминированного сжатия/трункейта), без лишних подсистем. Результат: набор review-документов с findings + конкретные изменения в коде/CI/релизе, чтобы проект был сфокусирован на MCP-router.

## Контекст
- **Документация:** docs/functions-router-summary.md, README.md, PLAN.md, REVIEW_ITERATION_4.md, tasks/0004-deterministic-context-extraction.md, tasks/0006-llm-token-budgeting-and-truncation.md
- **Целевые файлы:** docs/review/00-review-process.md, docs/review/01-inventory-and-scope-map.md, docs/review/02-repo-structure-findings.md, docs/review/03-router-findings.md, docs/review/04-mcp-layer-findings.md, docs/review/05-server-proxy-findings.md, docs/review/06-discovery-registry-findings.md, docs/review/07-executors-tools-findings.md, docs/review/08-ci-cd-release-findings.md, docs/review/09-consolidated-findings.md, docs/review/10-scope-reduction-plan.md, docs/architecture/mcp-smart-router.md, docs/configuration.md, README.md, PLAN.md, config.example.yaml, .github/workflows/ci.yml, .golangci.yml, Makefile, .dockerignore, .gitignore, internal/router/types.go, internal/router/tool_index.go, internal/router/tool_index_test.go, internal/router/prompt.go, internal/router/truncate.go, internal/router/truncate_test.go, internal/router/context_extractors.go, internal/router/context_extractors_test.go, internal/router/policy.go, internal/router/router.go, internal/router/output.go, internal/router/openrouter_client.go, internal/router/router_test.go, internal/router/router_integration_test.go, internal/registry/registry.go, pkg/mcp/protocol.go, pkg/mcp/transport.go, internal/server/server.go, internal/server/resources.go, internal/proxy/proxy.go, cmd/router/main.go, cmd/proxy/main.go, .goreleaser.yaml, docs/functions-router-summary.md

## TODO

### Phase 1: Baseline setup for review (фиксируем артефакты ревью)
- [x] **1.1** Создать шаблон ревью-процесса: критерии (архитектура, безопасность, DX, тесты, размер контекста, scope-fit), формат фиксации недостатков (severity, impact, fix, owner).
  - Файл: `docs/review/00-review-process.md`
- [x] **1.2** Добавить root Makefile для воспроизводимых проверок (make test, lint, fmt, vet). Зафиксировать команды, которые будут использоваться в ревью/CI.
  - Файл: `Makefile`
- [x] **1.3** Добавить конфиг линтера golangci-lint с базовыми правилами (govet, staticcheck, errcheck, gofmt, goimports) и исключениями только при явной причине.
  - Файл: `.golangci.yml`
- [x] **1.4** Добавить GitHub Actions workflow для CI: go test ./..., golangci-lint, загрузка coverage (если применимо). Должен работать на PR и push в main.
  - Файл: `.github/workflows/ci.yml`
- [x] **1.5** Добавить .dockerignore для исключения мусора из сборки/контекста Docker (особенно .worktrees, bin, stage* каталоги, coverage/out).
  - Файл: `.dockerignore`
- [x] **1.6** Добавить/обновить .gitignore (bin/, coverage.out, .env, временные файлы). Цель: не держать артефакты сборки в репозитории.
  - Файл: `.gitignore`

### Phase 2: Full project review (инвентаризация + выявление недостатков)
- [x] **2.1** Составить карту проекта: перечислить основные подсистемы (router/server/proxy/mcp/discovery/executor/tools/packages) и зачем они нужны; отметить, что входит/не входит в цель «умный MCP-router».
  - Файл: `docs/review/01-inventory-and-scope-map.md`
- [x] **2.2** Ревью репозиторной структуры на предмет раздутия контекста (дубликаты stage*/task*/.worktrees), рисков поддержки и влияния на понимание проекта. Зафиксировать рекомендации: что архивировать/вынести/удалить, что оставить.
  - Файл: `docs/review/02-repo-structure-findings.md`
- [x] **2.3** Ревью core-роутера: internal/router/* — корректность архитектуры, ответственность модулей, стабильность/детерминизм, тестируемость, риски утечек токенов/контекста. Зафиксировать недостатки и конкретные улучшения.
  - Файл: `docs/review/03-router-findings.md`
- [x] **2.4** Ревью MCP слоя: pkg/mcp/* — полнота протокола/транспорта, совместимость, ошибки, таймауты, ретраи, контракты. Зафиксировать недостатки и улучшения.
  - Файл: `docs/review/04-mcp-layer-findings.md`
- [x] **2.5** Ревью server/proxy: internal/server/*, internal/proxy/* — границы ответственности (сервер MCP vs роутер vs прокси), обработка ошибок, логирование, конфигурация, безопасность. Зафиксировать недостатки и улучшения.
  - Файл: `docs/review/05-server-proxy-findings.md`
- [x] **2.6** Ревью discovery/registry: internal/discovery/*, internal/registry/* — нужно ли для целевого продукта, где избыточно, какие зависимости/стоимость в контексте. Указать минимальный необходимый вариант для роутера.
  - Файл: `docs/review/06-discovery-registry-findings.md`
- [x] **2.7** Ревью executor/tools: internal/executor/*, internal/tool/*, internal/tools/* — оценить соответствие целям проекта. Зафиксировать план изоляции/удаления/вынесения (не реализация, а решение).
  - Файл: `docs/review/07-executors-tools-findings.md`
- [x] **2.8** Ревью CI/CD и релизов: .github/workflows/* и .goreleaser.yaml — риски, лишние артефакты, что публикуется, как обеспечить выпуск только MCP-router бинаря/пакета.
  - Файл: `docs/review/08-ci-cd-release-findings.md`
- [x] **2.9** Свести все findings в единый список (таблица): issue, severity, impact, proposed fix, file/module. Должно включать отдельный раздел по проблеме из docs/functions-router-summary.md (policy-block на tool discovery) и рекомендации (fallback/архитектурное решение).
  - Файл: `docs/review/09-consolidated-findings.md`

### Phase 3: Define target product (smart MCP router only) and align docs/config
- [x] **3.1** Обновить README: зафиксировать минимальный продукт (Smart MCP Router), входные/выходные контракты, что НЕ входит (executors, большие наборы встроенных tools, генераторы, stage-архивы).
  - Файл: `README.md`
- [ ] **3.2** Обновить PLAN.md: привести дорожную карту к цели «умный MCP-router для экономии контекста», добавить milestones (tool indexing, prompt minimization, deterministic truncation, caching).
  - Файл: `PLAN.md`
- [ ] **3.3** Добавить архитектурный документ: компоненты, поток запроса, где экономим контекст, где кэшируем, какие гарантии детерминизма. Включить раздел «Policy ограничения и как живем без tool-search».
  - Файл: `docs/architecture/mcp-smart-router.md`
- [ ] **3.4** Привести config.example.yaml к минимально необходимой конфигурации для роутера (модели/провайдер, токен-бюджеты, upstream MCP endpoints, cache TTL). Отдельно пометить deprecated/optional параметры.
  - Файл: `config.example.yaml`
- [ ] **3.5** Добавить документ по конфигурированию/запуску: как запустить router бинарь, какие env/config нужны, примеры запросов, troubleshooting (включая кейс policy-block).
  - Файл: `docs/configuration.md`

### Phase 4: Router changes to save context (tool selection + prompt minimization + deterministic truncation)
- [ ] **4.1** Уточнить/зафиксировать целевые типы роутера: минимальное представление Tool (имя/описание/входная схема/хэш схемы), запрос/ответ роутера, метрики использования контекста. Обеспечить совместимость с текущими вызовами.
  - Файл: `internal/router/types.go`
- [ ] **4.2** Добавить компонент индексации инструментов для уменьшения контекста: детерминированный подбор top-K tools по запросу (keyword scoring/heuristics), стабильная сортировка, ограничение по токенам/символам. Не использовать динамический tool-search внутри роутера.
  - Файл: `internal/router/tool_index.go`
- [ ] **4.3** Написать unit-тесты для tool index: детерминизм (одинаковый ввод -> одинаковый вывод), корректный top-K, корректное поведение при пустых/длинных описаниях, лимиты.
  - Файл: `internal/router/tool_index_test.go`
- [ ] **4.4** Переписать сборку промпта так, чтобы включались только выбранные tool schemas/описания. Добавить режимы: (a) only names+short desc, (b) names+schema for top-N. Обязательное: стабильный формат для кэширования и тестов.
  - Файл: `internal/router/prompt.go`
- [ ] **4.5** Усилить deterministic truncation: разделить бюджеты на system/instructions, chat history, tool descriptions/schemas, tool results. Добавить явные причины отрезания и метаданные для дебага.
  - Файл: `internal/router/truncate.go`
- [ ] **4.6** Добавить тесты на truncation/token budgeting: проверка, что при заданных бюджетах секции урезаются ожидаемо и детерминированно, без паник и без повреждения JSON/schema блоков.
  - Файл: `internal/router/truncate_test.go`
- [ ] **4.7** Обновить извлечение контекста: гарантировать детерминированность, убрать случайные/шумные части, добавлять только то, что нужно для выбора инструмента. Добавить опцию «exclude_tool_results» для экономии контекста в повторных шагах.
  - Файл: `internal/router/context_extractors.go`
- [ ] **4.8** Добавить тесты на context extractors: одинаковый вход -> одинаковый выход; корректное исключение/включение частей; лимиты на размер.
  - Файл: `internal/router/context_extractors_test.go`
- [ ] **4.9** Обновить policy роутера: запретить внутреннюю попытку tool discovery через локальные search/list шаги, если это может быть заблокировано. Реализовать понятный fallback: требовать tool list явным образом или брать из registry/cache.
  - Файл: `internal/router/policy.go`
- [ ] **4.10** Интегрировать tool index + truncation + policy в основной pipeline роутинга (router.go): (1) получить tools из registry, (2) выбрать top-K, (3) построить минимальный промпт, (4) получить решение, (5) вернуть план/ответ с метаданными по бюджету.
  - Файл: `internal/router/router.go`
- [ ] **4.11** Усилить output shaping: обеспечить, что роутер может возвращать (a) только tool-call plan, (b) plan + финальный ответ. Добавить структурированные поля для отладки (selected_tools_count, prompt_chars/tokens estimate).
  - Файл: `internal/router/output.go`
- [ ] **4.12** Обновить OpenRouter client: таймауты, ретраи, редактирование логов (не логировать полный контент/схемы целиком), и стабильная сериализация запросов для тестов.
  - Файл: `internal/router/openrouter_client.go`

### Phase 5: MCP integration & tool registry (без недоступного tool-search) + runtime wiring
- [ ] **5.1** Зафиксировать интерфейс registry для списка tools: получение tool descriptors из upstream MCP серверов + кэширование. Должно возвращать уже нормализованные структуры для tool index (без лишнего текста).
  - Файл: `internal/registry/registry.go`
- [ ] **5.2** Уточнить MCP protocol: добавить/проверить поддержку операций, необходимых роутеру (list tools, call tool). Обновить структуры/константы и комментарии по совместимости.
  - Файл: `pkg/mcp/protocol.go`
- [ ] **5.3** Усилить MCP transport: таймауты, контекст, корректная обработка ошибок upstream, лимиты на payload (чтобы не тащить большие схемы без надобности).
  - Файл: `pkg/mcp/transport.go`
- [ ] **5.4** Обновить server wiring: сервер должен уметь (a) принимать запрос на роутинг, (b) иметь доступ к registry tool list, (c) возвращать детерминированные ответы. Обновить зависимости так, чтобы лишние подсистемы не подтягивались.
  - Файл: `internal/server/server.go`
- [ ] **5.5** Обновить resources/endpoints сервера, чтобы был явный путь получения списка tools (или проксирование MCP list-tools), который не требует «поиска инструментов» внутри LLM роутера.
  - Файл: `internal/server/resources.go`
- [ ] **5.6** Уточнить поведение прокси: после решения роутера корректно проксировать вызов на выбранный upstream MCP server/tool, добавив защиту от циклов/повторных вызовов и контроль размера ответа.
  - Файл: `internal/proxy/proxy.go`
- [ ] **5.7** Добавить новый минимальный entrypoint для продукта «router»: отдельный cmd/router, который поднимает сервер и роутер (и только нужные зависимости).
  - Файл: `cmd/router/main.go`
- [ ] **5.8** Обновить/пометить существующий cmd/proxy как deprecated или сделать thin-wrapper вокруг cmd/router (без дублирования логики).
  - Файл: `cmd/proxy/main.go`

### Phase 6: Reduce non-core surface area + tests + release hardening
- [ ] **6.1** Добавить документ с планом изоляции/архивации лишних компонентов (executors/tools/discovery variants, stage* и .worktrees) для уменьшения контекста и поддержки. Включить пошаговый план (что переносим в отдельный репо/ветку/архив).
  - Файл: `docs/review/10-scope-reduction-plan.md`
- [ ] **6.2** Добавить/обновить интеграционные тесты роутера на кейс из docs/functions-router-summary.md: когда попытка tool discovery запрещена policy — роутер должен корректно работать от registry list-tools или вернуть понятную ошибку без падения.
  - Файл: `internal/router/router_integration_test.go`
- [ ] **6.3** Обновить unit-тесты основного роутера: проверить, что prompt включает только top-K tools и соблюдает бюджеты; добавить golden tests (ожидаемый промпт/структура).
  - Файл: `internal/router/router_test.go`
- [ ] **6.4** Привести .goreleaser.yaml к выпуску только целевого артефакта (cmd/router). Исключить публикацию лишних бинарей/пакетов, настроить ldflags/versioning, исключить мусорные директории из архивов.
  - Файл: `.goreleaser.yaml`
- [ ] **6.5** Обновить docs/functions-router-summary.md (или добавить рядом примечание) с итогом исправлений: как теперь работает tool listing/selection без запрещенного search_tools, какие ограничения остаются.
  - Файл: `docs/functions-router-summary.md`

## Статус
- **Создана:** 2025-12-28
- **Обновлена:** 2025-12-28
- **Статус:** В работе

## Заметки
Ключевой фокус: экономия контекста за счет (1) явного списка tools из registry (а не через LLM tool discovery), (2) детерминированного отбора top-K инструментов и сжатия схем, (3) строгих бюджетов и детерминированного truncation. Отдельно: репозиторий содержит множество stage*/task* и .worktrees с дубликатами — это должно быть отражено в findings и плане сокращения контекста (архивация/вынесение), но сами массовые перемещения файлов лучше выполнять отдельным PR после утверждения scope-reduction-plan.