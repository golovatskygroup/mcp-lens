# Задача: Project review + scope reduction to MCP smart router

## Описание
Провести ревью всего репозитория, зафиксировать недостатки и привести проект к целевому состоянию: он должен имплементировать только «умный роутер» для MCP, который экономит контекст (не делает tool introspection/поиск инструментов во время рантайма, не тянет лишние подсистемы, минимально включает схемы инструментов в промпт, поддерживает токен-бюджет/транкацию/детерминированное извлечение контекста). Результат: полный отчет по ревью + приоритизированный план исправлений + внедренные изменения в роутере/рантайме/конфиге/тестах/CI для соответствия целевой архитектуре.

## Контекст
- **Документация:** docs/functions-router-summary.md, docs/review/00-review-process.md, docs/review/01-inventory-and-scope-map.md, docs/review/02-repo-structure-findings.md, docs/review/03-router-findings.md, docs/review/04-mcp-layer-findings.md, docs/review/05-server-proxy-findings.md, docs/review/06-discovery-registry-findings.md, docs/review/07-executors-tools-findings.md, docs/review/08-ci-cd-release-findings.md, docs/review/09-consolidated-findings.md, tasks/0004-deterministic-context-extraction.md, tasks/0006-llm-token-budgeting-and-truncation.md
- **Целевые файлы:** PLAN.md, README.md, config.example.yaml, go.mod, go.sum, .golangci.yml, .github/workflows/ci.yml, cmd/router/main.go, cmd/proxy/main.go, internal/server/server.go, internal/proxy/proxy.go, internal/router/catalog.go, internal/router/catalog_config.go, internal/router/router.go, internal/router/policy.go, internal/router/policy_test.go, internal/router/prompt.go, internal/router/truncate.go, internal/router/router_test.go, internal/router/router_integration_test.go, internal/router/schema_summarizer.go, internal/router/schema_summarizer_test.go, docs/functions-router-summary.md, docs/router-tool-catalog.md, docs/review/00-review-process.md, docs/review/01-inventory-and-scope-map.md, docs/review/02-repo-structure-findings.md, docs/review/03-router-findings.md, docs/review/04-mcp-layer-findings.md, docs/review/05-server-proxy-findings.md, docs/review/06-discovery-registry-findings.md, docs/review/07-executors-tools-findings.md, docs/review/08-ci-cd-release-findings.md, docs/review/09-consolidated-findings.md, docs/review/10-target-architecture-mcp-smart-router.md, docs/review/11-deprecation-plan.md, packages/mcp-lens/README.md

## TODO

### Phase 1: Scope, критерии приемки и план ревью
- [ ] **1.1** Зафиксировать цель проекта (только MCP smart router) и критерии приемки: (a) роутер не использует tool discovery/search_tools в рантайме, (b) список tools и их JSON-схем подается явно (конфиг/внешний реестр), (c) в LLM-контекст попадает только top-K релевантных tools + сжатые схемы, (d) соблюдается токен-бюджет и транкация, (e) go test ./... проходит в CI. Описать процесс ревью, артефакты и формат findings (severity/effort).
  - Файл: `docs/review/00-review-process.md`
- [ ] **1.2** Обновить scope map: что остается (pkg/mcp, internal/router, минимальный server/transport для MCP), что объявляется out-of-scope/удаляется/замораживается (discovery, executors, registry, dev tools, npm package) и почему. Добавить раздел 'Decision log' (что удаляем сейчас, что позже).
  - Файл: `docs/review/01-inventory-and-scope-map.md`
- [ ] **1.3** Синхронизировать верхнеуровневый PLAN: этапы перехода к минимальному роутеру, риски (breaking changes), контрольные точки (M1: отчет; M2: запрет tool discovery; M3: отдельный бинарь роутера; M4: чистка зависимостей).
  - Файл: `PLAN.md`

### Phase 2: Инвентаризация репозитория и базовые метрики
- [ ] **2.1** Составить инвентарь исполняемых артефактов (cmd/*, bin/*), что реально собирается/публикуется, и какие пакеты тянут друг друга. Зафиксировать лишние/дублирующиеся компоненты и влияние .worktrees (это снапшоты стадий, не прод-код).
  - Файл: `docs/review/02-repo-structure-findings.md`
- [ ] **2.2** Проверить текущий статус качества: go test ./... (включая integration), покрытие/флейки, golangci-lint. Зафиксировать наблюдения и конкретные проблемные тесты/пакеты в отчете.
  - Файл: `docs/review/02-repo-structure-findings.md`
- [ ] **2.3** Аудит зависимостей Go: определить лишние зависимости для целевого 'только роутер'. Сформировать список кандидатов на удаление/замену. Добавить в findings ссылку на конкретные import-цепочки.
  - Файл: `docs/review/02-repo-structure-findings.md`
- [ ] **2.4** Аудит Node-пакета packages/mcp-lens: определить роль в целевой архитектуре. Если не нужен для smart router, пометить как deprecated/вынести из дефолтного пути сборки (описать решение в findings).
  - Файл: `packages/mcp-lens/README.md`

### Phase 3: Ревью по подсистемам и фиксация недостатков (findings)
- [ ] **3.1** Ревью internal/router: архитектура, маршрутизация, формирование промпта, выбор tools, токен-бюджет, транкация, стабильность вывода, тестируемость. Отдельно: где и почему появляется tool discovery, и как это ломается политикой (как в functions-router-summary).
  - Файл: `docs/review/03-router-findings.md`
- [ ] **3.2** Ревью MCP-слоя (pkg/mcp + internal/server): соответствие протоколу, где смешаны concerns (server vs router), где можно упростить для 'только роутер'. Зафиксировать изменения интерфейсов/структур, чтобы не тянуть лишние пакеты.
  - Файл: `docs/review/04-mcp-layer-findings.md`
- [ ] **3.3** Ревью server/proxy слоя (cmd/proxy, internal/proxy): нужен ли proxy для целевой поставки. Если нужен только router MCP server, определить что удалить/упростить и как изменится entrypoint.
  - Файл: `docs/review/05-server-proxy-findings.md`
- [ ] **3.4** Ревью discovery/registry: подтвердить, что для целевого решения (экономия контекста) динамическое discovery не требуется в рантайме. Зафиксировать риск/стоимость поддержки и конкретные точки интеграции, которые надо разорвать.
  - Файл: `docs/review/06-discovery-registry-findings.md`
- [ ] **3.5** Ревью executors/tools: определить, какие инструменты/исполнители не должны быть частью минимального роутера. Зафиксировать план: удалить, вынести, или оставить за build-tag/отдельным модулем.
  - Файл: `docs/review/07-executors-tools-findings.md`
- [ ] **3.6** Ревью CI/CD/Release: проверить, что pipelines не собирают/публикуют лишнее, и что есть проверки, релевантные smart router (tests, lint, build router binary). Зафиксировать недочеты и нужные правки.
  - Файл: `docs/review/08-ci-cd-release-findings.md`
- [ ] **3.7** Собрать консолидированный отчет: топ недостатков с severity (P0/P1/P2), причинами, последствиями (контекст/политики/надежность), и дорожной картой исправлений с оценкой effort.
  - Файл: `docs/review/09-consolidated-findings.md`

### Phase 4: Целевая архитектура 'MCP smart router' (без introspection) и дизайн-решения
- [ ] **4.1** Описать целевую архитектуру: минимальные пакеты и потоки данных (MCP request -> context extraction -> tool ranking -> prompt assembly with top-K schemas -> LLM call -> structured output). Отдельно: как подаются tools (явный catalog), как кэшируются схемы/сжатые представления, и как гарантируется отсутствие search_tools.
  - Файл: `docs/review/10-target-architecture-mcp-smart-router.md`
- [ ] **4.2** Описать стратегию сокращения репо: что удаляем сразу, что помечаем deprecated, что оставляем как optional. Указать конкретные пакеты/директории, влияние на сборку/релиз, и план миграции.
  - Файл: `docs/review/11-deprecation-plan.md`

### Phase 5: Реализация — явный Tool Catalog и запрет tool discovery (ключевое для экономии контекста)
- [ ] **5.1** Добавить абстракцию ToolCatalog (явный список доступных tools + схемы + метаданные). Реализация in-memory. В API предусмотреть: ListTools(), GetTool(name), TopKByQuery(query, k) (ранжирование может быть простым: ключевые слова/TF-IDF/эмбеддинги позже, но интерфейс должен позволять улучшение).
  - Файл: `internal/router/catalog.go`
- [ ] **5.2** Добавить загрузку ToolCatalog из конфигурации (YAML/JSON) с явным перечислением инструментов и их JSON Schema. Обновить пример конфига, включая параметр K (сколько tools включать в контекст) и лимиты токенов/символов на схемы.
  - Файл: `internal/router/catalog_config.go`
- [ ] **5.3** Обновить пример конфигурации: добавить секцию router.tool_catalog (источник: inline/file), router.top_k_tools, router.schema_budget, router.total_prompt_budget. Убедиться, что конфиг читается существующим entrypoint или добавить минимальный парсер.
  - Файл: `config.example.yaml`
- [ ] **5.4** Переподключить internal/router/router.go на ToolCatalog вместо любых механизмов 'поиска инструментов'. Удалить/обойти кодовые пути, генерирующие шаги типа search_tools. Гарантировать, что роутер работает при известном наборе tools без introspection.
  - Файл: `internal/router/router.go`
- [ ] **5.5** Добавить явный policy-guard: запретить планы/шаги, которые пытаются выполнить search_tools/tool listing. Если текущая архитектура использует планировщик — добавить валидацию плана и fail-fast с понятной ошибкой. Это должно закрыть кейс из docs/functions-router-summary.md.
  - Файл: `internal/router/policy.go`
- [ ] **5.6** Добавить тесты на policy-guard: (a) план с search_tools отклоняется, (b) нормальный план без introspection принимается. Тест должен быть детерминированный и быстрый.
  - Файл: `internal/router/policy_test.go`
- [ ] **5.7** Вынести/добавить сборку промпта так, чтобы в контекст попадали только top-K tools + сжатое описание/схема. Обновить prompt assembly и избежать вставки полного каталога (экономия контекста).
  - Файл: `internal/router/prompt.go`
- [ ] **5.8** Реализовать 'schema summarization/truncation': функция, которая берет JSON Schema и делает компактную форму (например: название, описание, required, properties с типами и короткими описаниями), плюс жесткие лимиты по символам/токенам. Покрыть тестами на предсказуемость и лимиты.
  - Файл: `internal/router/schema_summarizer.go`
- [ ] **5.9** Интегрировать schema summarization в prompt assembly и/или в ToolCatalog (кэшировать компактный вид). Добавить кеширование результата, чтобы не пересчитывать на каждый запрос.
  - Файл: `internal/router/catalog.go`
- [ ] **5.10** Проверить и усилить токен-бюджетирование и транкацию: убедиться, что (a) системные инструкции не съедают бюджет, (b) контекст/схемы урезаются первыми по правилам, (c) детерминированность транкации сохраняется. При необходимости обновить truncate utilities и их тесты.
  - Файл: `internal/router/truncate.go`

### Phase 6: Минимальный рантайм/entrypoint — отдельный бинарь роутера MCP
- [ ] **6.1** Добавить отдельный entrypoint cmd/router/main.go: поднимает MCP server, подключает internal/router и ToolCatalog из конфига. Бинарь должен быть минимальным: без discovery/executors/tools, только маршрутизация. Добавить флаги/ENV для пути к конфигу.
  - Файл: `cmd/router/main.go`
- [ ] **6.2** Пересмотреть cmd/proxy/main.go: либо (a) оставить как совместимость, но пометить deprecated и не использовать в README, либо (b) упростить/переименовать так, чтобы основной путь запуска был cmd/router. Зафиксировать решение и обновить код соответственно.
  - Файл: `cmd/proxy/main.go`
- [ ] **6.3** Убедиться, что internal/server/server.go не подтягивает лишние зависимости (executors/discovery/tools) для сценария smart router. При необходимости разделить инициализацию на минимальную (router-only) и расширенную (legacy).
  - Файл: `internal/server/server.go`

### Phase 7: Декомпозиция/изолирование лишних подсистем (чтобы проект был 'только роутер')
- [ ] **7.1** Разорвать зависимости роутера/сервер-роутера от discovery/registry/executor/tools в основном пути сборки: убрать импорты/инициализацию/регистрацию, которые не нужны для маршрутизации. Цель: cmd/router должен собираться и работать без этих пакетов.
  - Файл: `internal/proxy/proxy.go`
- [ ] **7.2** Пометить нерелевантные пакеты как deprecated (package comment + DEPRECATED.md с объяснением и ссылкой на целевую архитектуру). Это нужно, если удаление сразу слишком рискованно. Минимум: discovery/executor/tools/registry.
  - Файл: `internal/discovery/README.md`
- [ ] **7.3** После разрыва зависимостей — почистить go.mod/go.sum: удалить неиспользуемые зависимости (go mod tidy), зафиксировать изменения. Отдельно отметить, если некоторые зависимости остаются из-за тестов/legacy cmd/proxy.
  - Файл: `go.mod`

### Phase 8: Тесты и регрессии (включая кейс с blocked policy)
- [ ] **8.1** Добавить регрессионный тест: роутер не должен планировать/вызывать search_tools даже при запросе 'что доступно?'. Ожидаемое поведение: ответить, что introspection недоступна, и предложить либо список из явного каталога, либо инструкцию как его настроить (в зависимости от требований).
  - Файл: `internal/router/router_test.go`
- [ ] **8.2** Добавить интеграционный тест: при наличии каталога с N tools, роутер выбирает top-K и в промпт попадают только они, а общий размер промпта не превышает budget. Тест должен проверять измеримые инварианты (кол-во tools, лимиты).
  - Файл: `internal/router/router_integration_test.go`
- [ ] **8.3** Добавить тесты на schema summarizer: входные сложные schema -> стабильный компактный вывод + соблюдение лимитов. Зафиксировать golden/снапшоты при необходимости.
  - Файл: `internal/router/schema_summarizer_test.go`

### Phase 9: CI/CD, качество и документация использования
- [ ] **9.1** Обновить CI: добавить сборку cmd/router, запуск go test ./..., запуск golangci-lint. Убедиться, что pipeline не требует секретов для unit-тестов и не падает из-за legacy пакетов, не используемых router-only путем.
  - Файл: `.github/workflows/ci.yml`
- [ ] **9.2** Обновить правила линтера/исключения так, чтобы они соответствовали целевой минимальной кодовой базе (убрать исключения под legacy где возможно).
  - Файл: `.golangci.yml`
- [ ] **9.3** Обновить README: проект = MCP smart router, как запускать cmd/router, как настроить tool catalog, какие гарантии (без introspection), как достигается экономия контекста, и что помечено deprecated.
  - Файл: `README.md`
- [ ] **9.4** Обновить docs/functions-router-summary.md: описать исправленное поведение и почему теперь нет policy block (search_tools не используется). Добавить раздел 'How to debug tool availability' через явный каталог.
  - Файл: `docs/functions-router-summary.md`
- [ ] **9.5** Добавить отдельную документацию формата tool catalog (пример JSON/YAML, поля schema, description, examples) и рекомендации по уменьшению схем (чтобы экономить контекст).
  - Файл: `docs/router-tool-catalog.md`

## Статус
- **Создана:** 2025-12-28
- **Обновлена:** 2025-12-28
- **Статус:** В работе

## Заметки
Ключевой критерий: роутер не должен пытаться 'сам' узнавать доступные инструменты через search_tools/перечисление функций в рантайме (это ломается политикой и тратит контекст). Вместо этого — явный Tool Catalog (конфиг/внешний реестр) + top-K отбор + сжатие схем + жесткий token/char budget. Все изменения должны быть покрыты тестами, а CI должен собирать и проверять именно cmd/router как основной deliverable.