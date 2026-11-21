# Сторонние проекты

Эти компоненты устанавливаются отдельно. Используйте актуальную инструкцию для Codex, Antigravity или Claude Code в исходном репозитории. Ссылки не означают, что весь список обязателен.

## Процесс разработки и навыки

| Проект | Зачем нужен |
| --- | --- |
| [obra/superpowers](https://github.com/obra/superpowers) | Уточнение задачи, планы, разработка, отладка, ревью и проверка завершения. В моем окружении используется локальный набор; две регистрации плагина выключены. Выберите один способ установки. |
| [mattpocock/skills](https://github.com/mattpocock/skills) | Grilling для проверки идей и планов, domain-modeling для терминов и модели предметной области. |
| [Nikolay-Shirokov/cc-1c-skills](https://github.com/Nikolay-Shirokov/cc-1c-skills) | Создание и изменение объектов 1С, форм, макетов, расширений и обработок, работа с базами и веб-клиентом. Есть варианты для разных агентов и рантаймов. |
| [Graphify-Labs/graphify](https://github.com/Graphify-Labs/graphify) | Граф связей проекта и исследование кода. Устанавливаются CLI и навык по README. |
| [tt-a1i/archify](https://github.com/tt-a1i/archify) | Архитектурные и процессные диаграммы. |
| [tenfoldmarc/llm-council-skill](https://github.com/tenfoldmarc/llm-council-skill) | Совет моделей для разбора вопросов; нужна отдельная настройка провайдеров. |
| [nextlevelbuilder/ui-ux-pro-max-skill](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) | Проектирование, верстка и ревью UI/UX: стили, палитры, типографика, графики и дизайн-системы. Устанавливается через CLI (`ui-ux-pro-max-cli`), маркетплейс Claude Code или в Antigravity командой `agy plugin install`. |
| [openai/skills](https://github.com/openai/skills) | Публичные навыки и инструменты установки навыков для Codex. Состав может отличаться от встроенного набора приложения. |

## MCP

| Проект | Зачем нужен |
| --- | --- |
| [microsoft/playwright-mcp](https://github.com/microsoft/playwright-mcp) | Управление браузером и проверка интерфейсов. |
| [alonehobo/BslEdit](https://github.com/alonehobo/BslEdit) | Просмотр форм и макетов 1С через MCP установленного приложения. |
| [Regsorm/bsl-context](https://github.com/Regsorm/bsl-context) | Контекст типов и API платформы 1С. В моем окружении используется Rust-реализация. |
| [Regsorm/code-index-mcp](https://github.com/Regsorm/code-index-mcp) | Индексация и поиск по исходному коду. |

Серверы настраиваются локально. Конфиги клиентских баз, адреса, учетные данные и готовые config.toml или mcp_config.json сюда не входят. Встроенные инструменты приложения не нужно дублировать отдельным MCP-сервером.

## Плагины и приложения

- [OpenAI Plugins](https://github.com/openai/plugins) - публичный каталог плагинов Codex.
- [Antigravity Plugins](https://antigravity.google/docs/plugins) - плагины Antigravity через `Customizations > Marketplace` или `agy plugin install <имя-или-url>` (включая импорт совместимых плагинов из GitHub).
- [Telepath](https://github.com/Lemma-Company/telepath-plugin) - подключение Telegram через соответствующий плагин.
- GitHub, Google Workspace, документы, таблицы, презентации и другие приложения устанавливаются через каталог вашего агента и требуют отдельной авторизации. У части встроенных компонентов нет подтвержденного отдельного публичного репозитория.

Не копируйте чужой кэш плагинов и настройки аккаунта. Версии внешних проектов меняются; ориентируйтесь на их актуальные README, а не на номера из чужой установки.
