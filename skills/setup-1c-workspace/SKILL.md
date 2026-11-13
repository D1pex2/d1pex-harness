---
name: setup-1c-workspace
description: Клонирование репозитория 1С-проекта и настройка рабочего места разработчика - git, доступ к репозиторию, .v8-project.json, платформа 1С, OneScript и vanessa-runner, загрузка исходников в тестовую базу. Использовать, когда разработчик впервые берёт проект (репозиторий вида "как Эскиз", создан навыком init-1c-repo).
---

# /setup-1c-workspace — клонирование и настройка рабочего места

Помогает новому разработчику взять репозиторий 1С-проекта и подготовить машину к работе. Идёт по шагам, на каждом сначала проверяет, что уже сделано, и пропускает готовое.

## Правила

- Ничего не ставь в систему и не меняй глобальные настройки без согласия пользователя: git, OneScript, переменные PATH, `git config --global`, учётные данные.
- Пароли и токены только в `.v8-project.json` (он в `.gitignore`) и в Диспетчере учётных данных Windows. Не пиши их в отслеживаемые файлы, `.git/config`, URL remote, сообщения коммитов и в ответы.
- Не принимай автоматически ключ хоста SSH и не отключай защиту `allowUnsafeRemotes`: это решение пользователя.
- Загрузка в **общую** тестовую базу перекрывает чужую работу: сначала спроси, личная это база или общая, и если общая, пусть пользователь предупредит команду.
- К рабочей (эксплуатационной) базе не подключайся.

## Что узнать

Спрашивай только то, чего нет в `AGENTS.md`/`docs/workflow.md` репозитория или в ответах пользователя:
1. Адрес репозитория и куда клонировать (по умолчанию рядом с текущим каталогом).
2. Способ доступа: SSH (ключ) или HTTP/HTTPS (токен GitLab).
3. Тестовая база: сервер и имя, логин и пароль; своя или общая.
4. Нужны ли тесты и проверки (vanessa-runner, OneScript) — по умолчанию да.

## Порядок работы

### 1. Git

`Get-Command git`. Если нет, предложи `winget install --id Git.Git -e` (потребует прав администратора, запросит подтверждение) и после установки обнови PATH в сеансе:
```powershell
$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')
```
Проверь `git config user.name` и `user.email`; если пусто, спроси и задай для репозитория (`git config` без `--global`).

### 2. Клонирование

- **SSH**: если `Host key verification failed`, пусть пользователь сам выполнит `ssh -T git@host`, сверит отпечаток и ответит `yes`. «Permission denied» значит, что его ключ не добавлен в GitLab.
- **HTTP/HTTPS с токеном**: токен (Personal Access Token, `read_repository`, для работы `write_repository`) даёт пользователь. Сохрани его через `git credential approve` (протокол, хост, `username=oauth2`, пароль = токен), он попадает в Диспетчер учётных данных. Не вставляй токен в URL клона, чтобы он не остался в `.git/config`.
- Git Credential Manager блокирует HTTP: «Unencrypted HTTP is not recommended». Для локальной сети разрешение — `git config --global credential.http://<хост>.allowUnsafeRemotes true`: предложи пользователю и выполняй только с его согласия.

```powershell
git clone <URL> <каталог>
```
Проверь: `git -C <каталог> status -sb`, `git log --oneline -3`, наличие `src/cf`, `src/cfe`, `AGENTS.md`.

### 3. Платформа 1С

Найди установленные: `Get-ChildItem "C:\Program Files\1cv8\*\bin\1cv8.exe"`. Формат выгрузки в репозитории задаёт платформа (смотри `AGENTS.md` и `docs/workflow.md`, версия обычно указана в `.v8-project.example.json` и `autumn-properties.json`). Если нужной версии нет, скажи об этом пользователю: ставить платформу самому не нужно.

### 4. Реестр баз `.v8-project.json`

Скопируй `.v8-project.example.json` в `.v8-project.json`, заполни `v8path`, `server`/`ref`, `user`, `password`, `extensions` (имена и каталоги `src/cfe/<Имя>`), `default`. Если база подключена к хранилищу конфигурации, добавь `repository` (путь и пользователь; пароль только если есть) — без него платформа не принимает операции конфигуратора.

Проверь, что файл не отслеживается: `git check-ignore -v .v8-project.json`.

### 5. Инструменты (если нужны тесты и проверки)

Состав зависимостей задаёт `packagedef` (обычно `add`, `vanessa-automation-single`, `vanessa-runner`, для разработки `1testrunner`, `oneunit`) и требуемая версия среды (`ВерсияСреды`, обычно 2.0.0).

**OneScript.** Проверь `oscript -version`.
- `1.x` не подходит: vanessa-runner падает в библиотеке `decorator` с ошибкой «Обработчик получения представления должен иметь 2 параметра».
- Если версии нет или она ниже 2.0, предложи (с согласием пользователя) портативную установку, не затирая системную:
```powershell
$r = Invoke-RestMethod https://api.github.com/repos/EvilBeaver/OneScript/releases/latest   # tag_name, assets
$ver = $r.tag_name.TrimStart('v')
$d = "$env:LOCALAPPDATA\OneScript\$ver"; New-Item -ItemType Directory -Force $d | Out-Null
$z = "$env:TEMP\OneScript-$ver-win-x64.zip"
Invoke-WebRequest "https://github.com/EvilBeaver/OneScript/releases/download/v$ver/OneScript-$ver-win-x64.zip" -OutFile $z
Expand-Archive $z $d -Force          # внутри bin, lib
```
- Добавь `bin` в пользовательский PATH **впереди** старого (иначе `oscript` останется 1.x), с согласием:
```powershell
$b = "$env:LOCALAPPDATA\OneScript\$ver\bin"; $u = [Environment]::GetEnvironmentVariable('Path','User')
if ($u -notlike "*$b*") { [Environment]::SetEnvironmentVariable('Path', "$b;$u", 'User') }
```
 Для текущего сеанса: `$env:Path = "$b;" + $env:Path`. В уже открытых терминалах и VS Code PATH обновится после перезапуска.

**Зависимости проекта.** Из корня репозитория:
```powershell
opm install -l
```
Пакеты ставятся в `oscript_modules` (он в `.gitignore`). Проверка:
```powershell
oscript_modules\bin\vrunner.bat --help        # должна показать vanessa-runner 3.x и список команд
```
`vrunner version` командой не является: «Ошибка чтения параметров команды» для неё нормальна, используй `--help` или `-v`.

**PATH проекта.** Каталог `oscript_modules\bin` в PATH не добавляй: путь привязан к папке репозитория. Из корня запускай `oscript_modules\bin\vrunner.bat`. Хочет короткую команду `vrunner` — `opm install -g vanessa-runner` (глобально).

**Платформа в настройках vrunner.** В `autumn-properties.json` ключ `vrunner.v8version` должен совпасть с установленной платформой в виде `8.3.27` (а не `8.5`, если такой нет). Если не совпадает, предложи исправить и после согласия правь только это значение.

**Пробный запуск.** Безопасная проверка, что vrunner видит платформу, без трогания чужих баз: создать пустую локальную базу и удалить её.
```powershell
vrunner infobase init --ibconnection "/Fbuild/ib_smoke" --v8version 8.3.27
Remove-Item build\ib_smoke -Recurse -Force
```

**Файлы настроек тестов.** Если в `autumn-properties.json` есть ссылки на `tools/VAParams.json`, `tools/xUnitParams.json`, `tools/yaxunit.json`, `tools/syntax-check-excludes.txt`, а файлов нет, скажи об этом: сами тесты без них не запустить. Создавать их без просьбы не нужно.

### 6. Загрузка исходников в тестовую базу

Перед загрузкой уточни: база своя или общая; не открыт ли в ней Конфигуратор («Ошибка блокировки информационной базы для конфигурирования»).

```powershell
# конфигурация
powershell.exe -NoProfile -File "<db-load-xml.ps1>" -InfoBaseServer <сервер> -InfoBaseRef <база> -UserName "<логин>" -Password "<пароль>" -ConfigDir "<проект>\src\cf" -Mode Full
# каждое расширение
... -ConfigDir "<проект>\src\cfe\<Имя>" -Mode Full -Extension "<Имя>"
# затем обновление БД
powershell.exe -NoProfile -File "<db-update.ps1>" ...
```
Скрипты — навыки `1c-skills:db-load-xml` и `1c-skills:db-update`.

- Если база **под хранилищем**: «Загрузка невозможна: текущая конфигурация помещена в хранилище». Полная загрузка из XML недопустима. Варианты, их выбирает пользователь: отключить личную базу от хранилища, либо получить версию из хранилища `db-repo update` и сравнить с `src` (`db-dump-xml` во временную папку вне проекта, хеши файлов без `ConfigDumpInfo.xml`).
- Расширение может быть загружено, но «платформа его не применит», если нет соединения с хранилищем. Это предупреждение, а не ошибка.
- Если база пустая или новая, загрузка пройдёт без этих ограничений.

### 7. Проверка

- `git status` чистый, `vrunner --help` работает, платформа находит базу.
- По желанию: `vrunner validate syntax-check --ibconnection "/S<сервер>/<база>" --db-user ... --db-pwd ... --v8version 8.3.27`. Много замечаний «возможно ошибочное свойство/метод» бывает и в рабочей конфигурации — это шум, не поломка окружения.
- Сравнение с `src`: выгрузка базы во временную папку и сравнение хешей показывает, совпадает ли база с репозиторием.

### 8. Итог пользователю

Коротко: что установлено и настроено, что осталось (токен, доступ к хранилищу, согласие на установку), и как работать дальше по `docs/workflow.md`: ветка на задачу, правки в своей тестовой базе, выгрузка в `src`, коммиты, review.

## Подводные камни

- «Отказано в доступе» к сетевой шаре хранилища: пользователь один раз заходит в проводнике на `\\сервер\...` под своей учёткой или `net use`.
- `LF will be replaced by CRLF` при `git add` — предупреждение. Автокрлф не трогай без просьбы.
- PowerShell 5 и 7 по-разному читают кириллицу в путях; передавай `-Encoding UTF8` и кавычки для путей.
