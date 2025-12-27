# Задача: Обновить документацию Agent CLI

## Описание
Добавить документацию для Agent CLI — набора AI-ассистентов, устанавливаемых через `install.sh`. В README отсутствует информация об агентах и их использовании.

## Контекст
- **Документация:** `README.md`, `docs/`
- **Целевые файлы:** `README.md`, `docs/agent-cli.md` (новый)
- **Зависимости:** Agent CLI уже установлен и работает

## Текущие агенты (из `agent list`)

| Агент | Описание |
|-------|----------|
| `browser` | Browser QA Specialist — screenshots, DOM analysis, visual validation |
| `crawler` | Web Crawler — fetch URLs, search with Sonar Pro, crawl sites |
| `docs` | Documentation Generator — create comprehensive docs from web sources |
| `specialist` | Expert Advisor — UX/UI, security, performance, architecture advice |
| `code` | Code Agent — generate/modify code with documentation context |
| `task` | Task Creator — generate structured tasks using GPT o3 |
| `orchestrator` | Task Orchestrator — process coding tasks with feedback loop |

## TODO

### Phase 1: Обновить README.md
- [ ] **1.1** Добавить секцию "Agent CLI" в README
  - Краткое описание назначения
  - Список доступных агентов
  - Базовые примеры использования
  - Ссылка на подробную документацию
  - Файл: `README.md`

- [ ] **1.2** Добавить информацию об установке
  - `install.sh` устанавливает `agent` binary
  - Требования: `OPENROUTER_API_KEY`
  - Файл: `README.md`

### Phase 2: Создать docs/agent-cli.md
- [ ] **2.1** Структура документа
  - Обзор и архитектура
  - Требования (Python, playwright, etc.)
  - Переменные окружения
  - Файл: `docs/agent-cli.md`

- [ ] **2.2** Документация browser agent
  ```
  agent browser screenshot <url> -o file.png
  agent browser analyze <url> --check-console --check-a11y
  agent browser validate <url> --checks ux_ui,accessibility --quick
  ```
  - Опции: `--headless`, `--no-headless`, `--quiet`
  - Checks: `ux_ui`, `functionality`, `accessibility`
  - Файл: `docs/agent-cli.md`

- [ ] **2.3** Документация crawler agent
  ```
  agent crawler <url>
  agent crawler --search "query" --max-pages 5
  agent crawler <url> --crawl --process --model minimax
  ```
  - Опции: `--search`, `--crawl`, `--max-pages`, `--js`, `--process`
  - Модели: `sonar`, `minimax`, `gemini`, `gpt4`, `claude`
  - Файл: `docs/agent-cli.md`

- [ ] **2.4** Документация docs agent
  ```
  agent docs "topic" -o output.md
  agent docs "topic" --depth 3 --max-sources 25 --model claude
  agent docs --list
  agent docs --search "query"
  ```
  - Опции: `--depth`, `--max-sources`, `--model`, `--tags`
  - Index commands: `--list`, `--search`, `--paths`, `--export-index`
  - Файл: `docs/agent-cli.md`

- [ ] **2.5** Документация specialist agent
  ```
  agent specialist ux_ui "Review this" --file index.html
  agent specialist security --file app.js
  agent specialist ux_ui,accessibility "Check form" --file form.html
  agent specialist --list
  ```
  - Специалисты: `ux_ui`, `security`, `performance`, `architecture`, `accessibility`, `seo`, `frontend`
  - Опции: `--file`, `--code`, `--context`, `--json`
  - Файл: `docs/agent-cli.md`

- [ ] **2.6** Документация code agent
  ```
  agent code "task description" --doc docs.md --file target.py
  agent code "task" --doc api.md --doc patterns.md --file src/client.py
  agent code "task" --doc docs.md --file app.py --dry-run
  ```
  - Опции: `--doc` (multiple), `--file`, `--dry-run`, `--reasoning-output`
  - Модели: `gemini-3-flash-preview`, `relace-apply-3`
  - Файл: `docs/agent-cli.md`

- [ ] **2.7** Документация task agent
  ```
  agent task "Create landing page"
  agent task "Refactor API" --context src/api/
  agent task "Add caching" --docs docs/architecture.md --model o3
  ```
  - Опции: `--context`, `--docs`, `--workspace`, `--model`, `--dry-run`
  - Модели: `gpt5` (default), `o3`, `gpt4`, `gemini`, `claude`
  - Файл: `docs/agent-cli.md`

- [ ] **2.8** Документация orchestrator agent
  ```
  agent orchestrator --init
  agent orchestrator
  agent orchestrator --task TASK-001
  agent orchestrator --specialists ux_ui,security
  agent orchestrator --no-browser --no-specialists
  ```
  - Опции: `--task`, `--max-iterations`, `--specialists`, `--no-browser`, `--dry-run`
  - Auto-specialists по типу файла
  - Workflow описание
  - Файл: `docs/agent-cli.md`

### Phase 3: Примеры и use-cases
- [ ] **3.1** Добавить практические примеры
  - Workflow: research → docs → code
  - Workflow: task → orchestrator
  - Workflow: code → specialist → browser validation
  - Файл: `docs/agent-cli.md`

- [ ] **3.2** Troubleshooting секция
  - Частые ошибки
  - Проверка API ключей
  - Установка зависимостей (playwright)
  - Файл: `docs/agent-cli.md`

### Phase 4: Обновить .env.example
- [ ] **4.1** Добавить все переменные для Agent CLI
  - `OPENROUTER_API_KEY` — обязательно
  - Опциональные переменные для моделей
  - Файл: `.env.example`

## Статус
- **Создана:** 2025-12-28
- **Обновлена:** 2025-12-28
- **Статус:** Новая

## Заметки

### Доступные модели по агентам
| Агент | Модели |
|-------|--------|
| crawler | sonar, minimax, gemini, gpt4, claude |
| docs | gemini (default), minimax, gpt4, claude |
| task | gpt5 (default), o3, gpt4, gemini, claude |
| code | gemini-3-flash-preview, relace-apply-3 |

### Auto-specialist mapping (orchestrator)
| Файлы | Специалисты |
|-------|-------------|
| `.html`, `.css` | ux_ui, accessibility |
| `.js` | security, frontend |
| `.tsx`, `.jsx` | ux_ui, security, frontend |
| `.py`, `.go` | security, architecture |

## Критерии приемки
- [ ] README содержит секцию Agent CLI с примерами
- [ ] `docs/agent-cli.md` создан с полной документацией всех агентов
- [ ] Каждый агент имеет примеры использования
- [ ] `.env.example` обновлен
- [ ] Документация соответствует актуальному `--help` выводу
