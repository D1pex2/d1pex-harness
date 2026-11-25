# d1pex-harness

### Мои правила и навыки для Codex, Antigravity и Claude Code

[![Codex](https://img.shields.io/badge/Codex-primary-111827)](https://github.com/openai/codex)
[![Antigravity](https://img.shields.io/badge/Antigravity-supported-4285F4)](https://antigravity.google/docs/getting-started)
[![Claude Code](https://img.shields.io/badge/Claude_Code-compatible_skills-D97757)](https://code.claude.com/docs/en/overview)
[![1C](https://img.shields.io/badge/1C-development-F7D44C)](skills/1c-development-rules/SKILL.md)

Моя обвязка для установки на другой мой ПК или ПК коллеги: личные правила работы агента, собственные навыки и дополнения, хуки подключения правил 1С и ссылки на сторонние инструменты. Основной сценарий - разработка и сопровождение 1С.

Здесь только то, что нужно для использования. Сторонние наборы устанавливаются нативно из своих репозиториев. Мои навыки копируются целыми каталогами. Установщика и готового конфига с моей машины нет.

[Установка для агента](INSTALL.md) | [Правила](AGENTS.md) | [Мои навыки](#мои-навыки) | [Сторонние проекты](SOURCES.md)

## Быстрый старт

Передайте агенту:

```text
Настрой d1pex-harness по инструкции:
https://github.com/D1pex2/d1pex-harness/blob/main/INSTALL.md

Установи нужные сторонние компоненты нативно по ссылкам.
Перенеси мои правила и выбранные локальные навыки.
Сохрани существующие настройки и проверь результат.
```

Или начните самостоятельно:

```bash
git -c core.longpaths=true clone https://github.com/D1pex2/d1pex-harness.git
cd d1pex-harness
```

Дальше следуйте [INSTALL.md](INSTALL.md): агент проверит зависимости на целевом ПК, установит выбранные внешние наборы, перенесет локальные файлы и проверит результат. Для Codex, Antigravity и Claude Code отличаются каталоги правил, навыков и регистрация хуков.

## Как устроено

```text
d1pex-harness/
|-- AGENTS.md        Мои правила без персональных путей
|-- skills/          27 локальных навыков с нужными ресурсами
|-- hooks/           Подключение правил 1С для Codex и Antigravity
|-- rules/           Протокол самопроверки
|-- SOURCES.md       Чужие навыки, плагины и MCP: ссылки и назначение
|-- INSTALL.md       Инструкция установки для агента
`-- NOTICE.md        Авторство и атрибуция
```

Правила задают границы работы. Навыки описывают способы решения задач. Хуки подключают предметные правила в нужный момент. MCP дает доступ к внешним инструментам.

Скрипты и справочники внутри навыков оставлены там, где они нужны для работы: проверки BSL и запросов, поиск API БСП, шаги Vanessa и другие предметные операции. Служебной обвязки для упаковки и установки окружения нет.

## Мои навыки

| Направление | Что использовать |
| --- | --- |
| Правила и архитектура 1С | [1c-development-rules](skills/1c-development-rules/SKILL.md), [1c-architecture-patterns](skills/1c-architecture-patterns/SKILL.md) |
| BSL и ревью | [1c-bsl-validate](skills/1c-bsl-validate/SKILL.md), [1c-code-review](skills/1c-code-review/SKILL.md), [проверка эквивалентности](skills/1c-refactor-equivalence-check/SKILL.md) |
| Запросы и метаданные | [составление запросов](skills/composing-1c-queries/SKILL.md), [валидация](skills/1c-query-validate/SKILL.md), [оптимизация](skills/1c-query-optimization/SKILL.md), [индекс конфигурации](skills/1c-config-index/SKILL.md) |
| БСП и ЗУП | [API БСП](skills/1c-bsp-api/SKILL.md), [паттерны БСП](skills/1c-ssl-patterns/SKILL.md), [команды](skills/1c-bsp-command/SKILL.md), [регистрация](skills/1c-bsp-registration/SKILL.md), [кадровый API ЗУП](skills/zup-hr-api-reference/SKILL.md) |
| Обработки и расширения | [полный цикл EPF](skills/1c-epf-full-cycle/SKILL.md), [полный цикл CFE](skills/1c-cfe-full-cycle/SKILL.md) |
| Хранилище и поддержка | [1c-storage-ops](skills/1c-storage-ops/SKILL.md), [1c-support-state](skills/1c-support-state/SKILL.md) |
| Проверка интерфейса | [веб-сессия 1С](skills/1c-web-session/SKILL.md), [шаги Vanessa](skills/1c-vanessa-steps/SKILL.md) |
| Настройка проектов | [init-1c-repo](skills/init-1c-repo/SKILL.md), [setup-1c-workspace](skills/setup-1c-workspace/SKILL.md), [setup-codex-env](skills/setup-codex-env/SKILL.md), [setup-antigravity-env](skills/setup-antigravity-env/SKILL.md) |
| Дополнения | [superpowers-extras](skills/superpowers-extras/SKILL.md), [Helium](skills/helium-browser/SKILL.md), [Open WebUI](skills/openwebui-admin-via-browser/SKILL.md) |

Полный состав виден в [skills/](skills). Перед использованием читайте SKILL.md: отдельным навыкам нужны PowerShell, Python, платформа 1С или сторонний набор 1C Skills.

## Хуки

Общий детектор [one-c-detector.js](hooks/one-c-detector.js) 1.1.0 работает самостоятельно и распознает проекты, файлы и MCP 1С для Codex, Antigravity и Claude Code.

Для Codex ([one-c-rules.js](hooks/one-c-rules.js) 1.1.0, шаблон [hooks.json](hooks/hooks.json)):

- `SessionStart`: доставить правила для проекта 1С при старте.
- `PreToolUse`: распознать работу с 1С и при необходимости добавить полный текст правил.

Для Claude Code (тот же [one-c-rules.js](hooks/one-c-rules.js), шаблон [hooks.claude.json](hooks/hooks.claude.json)):

- `SessionStart`: доставить правила для проекта 1С при старте.
- `UserPromptSubmit`: один раз на запрос распознать 1С по рабочей папке и путям в тексте запроса.
- `PreToolUse` только для правок файлов и MCP BSL Context, Code Index и BslEdit: правила гарантированно приходят до правки BSL вне проекта 1С. Чтение, поиск и команды оболочки хук не запускают, это экономит около 0.13 с на каждый такой вызов.

Для Antigravity ([one-c-rules-antigravity.js](hooks/one-c-rules-antigravity.js) 1.0.0, шаблон [hooks.antigravity.json](hooks/hooks.antigravity.json)):

- `SessionStart`: доставить правила для проекта 1С при старте через `injectSteps`.
- `PreInvocation`: распознать работу с 1С и при необходимости добавить полный текст правил перед ходом модели.
- `PostToolUse`: read-only фиксация активности 1С (по файлам `.bsl`/`.v8-project.json` и MCP 1С) без влияния на разрешения инструментов, чтобы следующий `PreInvocation` сразу подключил правила.

В обычном проекте хуки молчат. Одинаковые правила доставляются один раз на сессию, после нового старта или изменения текста доставляются снова. При `EVENTLOG_AUTORUN` хуки отключены. Состояние доставки создается локально и в репозиторий не входит.

Наблюдатель и проверки Stop не используются. Тестирование результата и обновление session.md задаются правилами и навыками.

## Сторонние компоненты

Основа процесса - [Superpowers](https://github.com/obra/superpowers). Операции с исходниками и базами 1С - [1C Skills](https://github.com/Nikolay-Shirokov/cc-1c-skills). Разбор планов и предметной области - [Matt Pocock Skills](https://github.com/mattpocock/skills).

Для инструментов использую Playwright MCP, BSL Context, Code Index и BslEdit. Остальные навыки подключаются по задаче. Все ссылки с пояснениями собраны в [SOURCES.md](SOURCES.md).

## Переносимость

Набор синхронизирован с моим окружением на 21.11.2026. Персональные пути и привязки к конкретной установке убраны, клиентские подключения и секреты не включены. Изменения на машине не попадают сюда автоматически.

В Codex используются `AGENTS.md`, `~/.agents/skills/` и хук `one-c-rules.js`. В Antigravity используются `~/.gemini/config/AGENTS.md` (или проектный `AGENTS.md`), `~/.gemini/config/skills/` (или `<проект>/.agents/skills/`), `~/.gemini/config/mcp_config.json` и хук `one-c-rules-antigravity.js`. В Claude Code используются `~/.claude/CLAUDE.md` со ссылкой на общий AGENTS.md, `~/.claude/skills/` и хук `one-c-rules.js` с шаблоном `hooks.claude.json`.

История main начинается с текущего очищенного снимка. Старые копии по SHA, кэшам, PR или форкам GitHub могут оставаться доступными.

Авторство и лицензии компонентов: [NOTICE.md](NOTICE.md).
