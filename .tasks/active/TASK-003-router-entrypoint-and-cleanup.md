# Задача: Router Entrypoint и Cleanup (завершение TASK-002)

## Описание
Завершить реализацию MCP Smart Router: создать минимальный entrypoint (`cmd/router`), очистить зависимости от неиспользуемых подсистем, обновить CI/CD и документацию. Это продолжение TASK-002, где уже реализованы ToolCatalog, policy guard и schema compression.

## Контекст
- **Предыдущая задача:** TASK-002-project-review---scope-reducti.md (Phase 5 реализована)
- **Документация:** docs/review/TASK-002-implementation-summary.md, docs/review/00-review-process.md, docs/review/01-inventory-and-scope-map.md
- **Целевые файлы:** cmd/router/main.go, cmd/proxy/main.go, internal/server/server.go, .github/workflows/ci.yml, README.md, docs/router-tool-catalog.md, go.mod, go.sum

## TODO

### Phase 6: Минимальный рантайм/entrypoint — отдельный бинарь роутера MCP
- [ ] **6.1** Создать `cmd/router/main.go`: минимальный entrypoint для smart router
  - Поднимает MCP server через `internal/server`
  - Загружает ToolCatalog из конфига (inline или file)
  - Подключает router с top-K selection и schema compression
  - Флаги/ENV: `--config`, `MCP_LENS_CONFIG_PATH`
  - Без discovery/executors/tools (только маршрутизация)
  - Файл: `cmd/router/main.go`
- [ ] **6.2** Обновить `cmd/proxy/main.go`:
  - Вариант A: Пометить как deprecated с комментарием, что основной путь — `cmd/router`
  - Вариант B: Сделать thin wrapper вокруг router с обратной совместимостью
  - Зафиксировать решение и обновить код
  - Файл: `cmd/proxy/main.go`
- [ ] **6.3** Убедиться, что `internal/server/server.go` не тянет лишние зависимости для router-only пути
  - Разделить инициализацию на минимальную (router-only) и расширенную (legacy)
  - Или добавить build tags для условной компиляции
  - Файл: `internal/server/server.go`

### Phase 7: Декомпозиция/изолирование лишних подсистем
- [ ] **7.1** Разорвать зависимости router/server от discovery/registry/executor/tools в router-only пути
  - Убрать импорты/инициализацию discovery из router path
  - Убрать импорты executor из router path (если не нужны)
  - Цель: `cmd/router` собирается без этих пакетов
  - Файл: `internal/server/server.go`, `internal/tools/router.go`
- [ ] **7.2** Пометить нерелевантные пакеты как deprecated
  - Добавить package comment `// Deprecated: ...` в `internal/discovery/`
  - Создать `internal/discovery/DEPRECATED.md` с объяснением
  - Аналогично для executor/tools если они не нужны для router
  - Файл: `internal/discovery/README.md` или `internal/discovery/DEPRECATED.md`
- [ ] **7.3** Почистить `go.mod/go.sum`
  - Запустить `go mod tidy`
  - Проверить, какие зависимости остаются из-за тестов/legacy cmd/proxy
  - Зафиксировать изменения
  - Файл: `go.mod`, `go.sum`

### Phase 9: CI/CD, качество и документация использования
- [ ] **9.1** Обновить CI workflow
  - Добавить сборку `cmd/router` бинаря
  - Запуск `go test ./...` (должен проходить без секретов для unit-тестов)
  - Запуск `golangci-lint`
  - Убедиться, что pipeline не падает из-за legacy пакетов
  - Файл: `.github/workflows/ci.yml`
- [ ] **9.2** Обновить правила линтера
  - Убрать исключения для legacy пакетов где возможно
  - Обновить `.golangci.yml` под минимальную кодовую базу
  - Файл: `.golangci.yml`
- [ ] **9.3** Обновить README
  - Проект = MCP smart router
  - Как запускать `cmd/router`
  - Как настроить tool catalog
  - Какие гарантии (без introspection)
  - Как достигается экономия контекста
  - Что помечено deprecated
  - Файл: `README.md`
- [ ] **9.4** Обновить `docs/functions-router-summary.md`
  - Убедиться, что раздел Resolution актуален
  - Добавить примеры использования ToolCatalog API
  - Файл: `docs/functions-router-summary.md`
- [ ] **9.5** Создать документацию формата tool catalog
  - Пример JSON/YAML формата каталога
  - Описание полей (name, description, source, inputSchema)
  - Рекомендации по уменьшению схем
  - Примеры конфигурации
  - Файл: `docs/router-tool-catalog.md`

### Phase 10: Интеграция ToolCatalog в существующий router handler
- [ ] **10.1** Обновить `internal/tools/router.go`
  - Использовать `PlanWithCatalog()` вместо `Plan()` когда доступна конфигурация
  - Загружать RouterConfig из конфига (если есть)
  - Применять top-K selection и schema compression
  - Файл: `internal/tools/router.go`
- [ ] **10.2** Добавить загрузку конфигурации router
  - Расширить `cmd/proxy/main.go` или создать отдельный config loader
  - Парсить `router` секцию из YAML
  - Передавать RouterConfig в router handler
  - Файл: `cmd/proxy/main.go` или `internal/config/router_config.go`

## Статус
- **Создана:** 2025-12-28
- **Обновлена:** 2025-12-28
- **Статус:** В работе

## Заметки
- Эта задача завершает TASK-002, реализуя оставшиеся фазы (6, 7, 9)
- Основная цель: получить минимальный `cmd/router` бинарь, который работает без discovery/executor зависимостей
- Важно сохранить обратную совместимость для существующих пользователей `cmd/proxy`
- Все изменения должны быть покрыты тестами
- CI должен собирать и проверять `cmd/router` как основной deliverable

## Критерии приемки
- [ ] `cmd/router` собирается и запускается без ошибок
- [ ] `go test ./...` проходит в CI без внешних зависимостей для unit-тестов
- [ ] `cmd/router` не требует discovery/executor пакетов для базовой работы
- [ ] README обновлен с инструкциями по использованию router
- [ ] Документация tool catalog создана
- [ ] CI собирает `cmd/router` бинарь
