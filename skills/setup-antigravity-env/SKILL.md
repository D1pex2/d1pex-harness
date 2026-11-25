---
name: setup-antigravity-env
version: "1.0.0"
description: Установка, обновление и проверка среды Antigravity, навыков, плагинов, хуков и MCP без замены действующих настроек пользователя.
disable-model-invocation: true
---

# Настройка среды Antigravity

1. Проверь `agy --version`, `agy plugin list --json`, `agy plugin validate --help`. Используй синтаксис установленной версии. Приложение Antigravity App и CLI обновляй их штатным механизмом.
2. Определи, что уже установлено в `~/.gemini/config/` и в рабочем проекте `.agents/`. Согласованный состав выполняй без повторных разрешений. Перед заменой настроек сохраняй резервную копию в защищенном каталоге пользователя. Не выводи заголовки авторизации и токены.
3. Для Git, Node.js, Python и платформы 1С проверь фактическую потребность задачи. Не ставь Java ради bsl-context, если используется Rust-реализация. Платформу 1С автоматически не устанавливай и не выбирай за пользователя серверную версию.

## Навыки и плагины

- Предпочитай один источник каждого набора. Глобальные навыки Antigravity лежат в `~/.gemini/config/skills/`, проектные - в `<проект>/.agents/skills/`, плагины - в `~/.gemini/config/plugins/`.
- Не создавай дубли одного навыка одновременно в `~/.gemini/config/skills/` и внутри плагина в `~/.gemini/config/plugins/<имя>/skills/`. При смене источника сначала сравни версии и локальные правки.
- Сторонние наборы с манифестом плагина ставь штатно через `agy plugin install <url-или-имя>` или через `Customizations > Marketplace`. Не держи постоянно включенными тяжелые плагины, если они не нужны в текущих задачах (например, `ui-ux-pro-max` для UI или `chrome-devtools-plugin`, если уже используется встроенный `browser` или `playwright` MCP).
- Не переноси в Antigravity навыки-обертки и оркестрацию из Codex/Claude Code, дублирующие встроенные механизмы (`invoke_subagent`, `swarm`, `run_workflow`, `update_steps`, `/grill-me`): `using-superpowers`, `superpowers-extras`, `diagnosing-superpowers`, `using-git-worktrees`, `dispatching-parallel-agents`, `subagent-driven-development`, `writing-plans`, `executing-plans`, `finishing-a-development-branch`, `grill-me`, `grill-with-docs`, `task-observer`, `setup-codex-env`. Из Superpowers имеет смысл переносить только методологию (`brainstorming`, `grilling`, `systematic-debugging`, `test-driven-development`, `verification-before-completion`, `requesting-code-review`, `receiving-code-review`).
- Не редактируй `~/.gemini/config/config.json` вручную: включение, отключение и удаление плагинов выполняются через `agy plugin enable/disable/uninstall` или интерфейс `Customizations > Installed`.
- Встроенные плагины Antigravity не удаляются; при необходимости их можно только отключить в настройках.

## MCP

Сначала прочитай существующий `~/.gemini/config/mcp_config.json` (поле `mcpServers`) и проверь, какие серверы уже подключены через плагины (`~/.gemini/config/plugins/*/mcp_config.json`). Сохраняй действующие серверы; добавляй только согласованные подключения.

Antigravity поддерживает транспорты `stdio` (`command`, `args`, `env`) и Streamable HTTP (`url`). Устаревший HTTP+SSE не поддерживается; для таких серверов нужен stdio-мост.

Для bsl-context выбери реализацию по существующей конфигурации. В текущем окружении используется `bsl-context-rs.exe` с `--config`. Перед изменением формата прочитай README именно установленной версии. Источники имен конфигураций бери из существующих индексов code-index; проверяй `symbol_sources_status` и состояние конкретного репозитория. Не подменяй Rust-сервер старым JAR.

Список клиентских баз не является частью установочного навыка и не коммитится в репозиторий. Адреса, алиасы, IP и секреты бери из настроек выбранного проекта или от пользователя. Тестовую базу обозначай суффиксом `-test`. Не одобряй автоматически все инструменты базы: права чтения и записи различаются. Секреты не вставляй в примеры, журналы или файлы навыка.

## Правила и хуки

Для Antigravity используй адаптированный файл [antigravity.md](../../antigravity.md) (без разделов Codex про `Astra`, `EVENTLOG_AUTORUN` и `superpowers-extras`, с вшитым контролем качества). Сохраняй текущий `~/.gemini/config/AGENTS.md` (или проектный `AGENTS.md`): не заменяй его целиком чужой копией, изменения правил показывай как точечные различия. Файлы внутри `~/.gemini/config/rules/*.md` и `.agents/rules/*.md` загружаются Antigravity только при наличии YAML frontmatter с полем `trigger:`; standalone `AGENTS.md` работает без frontmatter.

Пользовательские хуки `one-c-rules-antigravity.js` и `one-c-detector.js` подключают правила 1С по признакам проекта, файлам и MCP 1С без списка клиентов. В `hooks.json` регистрируются `SessionStart`, `PreInvocation` и read-only `PostToolUse`. Рабочий каталог команды хука в Antigravity совпадает с каталогом `hooks.json`, поэтому относительный путь `./hooks/one-c-rules-antigravity.js` работает переносимо. При переносе окружения проверь хуки на временном проекте.

## Проверка результата

Проверь разбор JSON (`mcp_config.json`, `hooks.json`), существование исполняемых файлов, вывод `agy plugin list --json` и `agy plugin validate` для установленных плагинов. Для измененных MCP проверь живое соединение безопасными read-only вызовами, а не только наличие записи в конфигурации. Напомни, что новые навыки и хуки подхватываются со следующего хода или в новом чате. Базы 1С проверяй только в согласованном объеме.

## Совместимость запускателей 1С

Перед выбором OneScript проверь минимальную версию установленного пакета vanessa-runner. Используй совместимый интерпретатор, найденный на машине пользователя. Не заменяй глобальный PATH, если он нужен старым пакетам. После восстановления oscript_modules проверь bat-файлы: пакетный менеджер может перегенерировать их. Проверка: локальный `vrunner.bat --version`.

Testpilot и другие вспомогательные инструменты используй из существующей среды пользователя либо установи по их документации в согласованном объеме. Не подставляй пути с машины автора навыка. Для bsl-context используй платформу конкретного проекта и его индекс; новый MCP для этого не нужен.
