---
name: helium-browser
description: Управлять живым браузером Helium пользователя с его профилем и логинами. Использовать, когда просят "открой сайт", "зайди на", "посмотри страницу", "залогинься", "заполни форму", "нажми кнопку на сайте", "сделай скриншот страницы", "вытащи данные с сайта" — а также когда задача требует доступа к залогиненному аккаунту (my.itmo.ru, почта, госуслуги, банк). Не поднимать свой браузер и не делать копию профиля.
---

# Живой браузер Helium (Windows)

Работаем в настоящем браузере пользователя: его профиль, его куки, его история.
Отдельный профиль не нужен и пользователь его не хочет. Helium — форк Chromium
(сейчас 154), запрет Chromium 136+ на `--remote-debugging-port` для дефолтного
user-data-dir он **не** применяет, поэтому живой профиль цепляется напрямую —
под Windows проверено на настоящем профиле.

Инструментов два. `pagectl` — основной, он не выполняет JS на странице.
`rodkit` — запасной, у него есть `eval`, и именно поэтому он оставляет следы.

## Где что лежит

- Helium: `%LOCALAPPDATA%\imput\Helium\Application\chrome.exe`. Процесс
  называется `chrome.exe`, как и у Google Chrome, — отбирать только по пути.
- `pagectl` и `rodkit` — в `%USERPROFILE%\go\bin`, он в PATH. Исходники —
  `rodkit/` рядом с этим SKILL.md, пересборка в этом каталоге: `go install ./cmd/...`.
- Сессия — `%USERPROFILE%\.rodkit\session.json`. Её пишет `pagectl`, руками не
  трогать.

## Какой оболочкой

Команды ниже — для PowerShell. Ловушки, в которые уже попадали:

- **Git Bash** (инструмент Bash) переписывает аргументы, начинающиеся с `/`:
  XPath `//button` доезжает как `/button`, путь `/booking/42` — как
  `C:/Program Files/Git/booking/42`. Если всё же bash — ставить
  `MSYS_NO_PATHCONV=1` перед командой.
- **Windows PowerShell 5.1** выбрасывает `"` внутри аргументов нативных программ:
  `'//a[@id="x"]'` доезжает как `//a[@id=x]`. Внутри селекторов, XPath и JS —
  одинарные кавычки: `pagectl find "//a[@id='x']"`.
- `curl` в PowerShell 5.1 — это `Invoke-WebRequest`; настоящий — `curl.exe`.

## Шаг 1. Поднять сессию

```powershell
pagectl tabs
```

Печатает вкладки — переходить к шагу 2. Браузер на `127.0.0.1:9222` pagectl
находит сам и сам переподключается, если Helium перезапускали.

«порт отладки 127.0.0.1:9222 не отвечает» — Helium запущен без порта.
**Спросить пользователя**, можно ли перезапустить браузер: окна закроются и
через несколько секунд откроются снова с теми же вкладками, но несохранённое на
страницах (набранный текст в формах) пропадёт. После согласия:

```powershell
$helium = "$env:LOCALAPPDATA\imput\Helium\Application\chrome.exe"
function Get-Helium { Get-Process chrome -ErrorAction SilentlyContinue | Where-Object Path -eq $helium }
# taskkill без /F шлёт окнам WM_CLOSE: Helium закрывается как по крестику и сохраняет сессию.
Get-CimInstance Win32_Process -Filter "Name='chrome.exe'" |
  Where-Object { $_.ExecutablePath -eq $helium -and $_.CommandLine -notmatch '--type=' } |
  ForEach-Object { taskkill /PID $_.ProcessId | Out-Null }
$t = (Get-Date).AddSeconds(20); while ((Get-Helium) -and (Get-Date) -lt $t) { Start-Sleep -Milliseconds 300 }
if (Get-Helium) { throw 'Helium не закрылся за 20 с' }
Start-Process $helium -ArgumentList '--remote-debugging-port=9222', '--restore-last-session', '--disable-backgrounding-occluded-windows'
$t = (Get-Date).AddSeconds(30)
do { Start-Sleep -Milliseconds 500
     try { Invoke-RestMethod http://127.0.0.1:9222/json/version -TimeoutSec 2 | Out-Null; $up = $true } catch { $up = $false }
} until ($up -or (Get-Date) -gt $t)
pagectl connect
```

Готово, когда `pagectl connect` печатает `Chrome/…, pid N`, а `pagectl tabs` —
вкладки пользователя.

Флаги не косметические:

- `--restore-last-session` возвращает вкладки. Без него Helium с настройками по
  умолчанию открывается с пустой вкладкой — проверено.
- `--disable-backgrounding-occluded-windows`: без него окно, перекрытое другими
  окнами (тем же терминалом), Windows считает невидимым — страница перестаёт
  рисовать кадры, а движение мыши Chrome подтверждает только на кадре, и
  `click`/`type` висят. Цена — перекрытое окно рисуется, пока открыт порт.

Если Helium не закрылся за 20 с — скорее всего, висит диалог «Покинуть сайт?».
Попросить пользователя ответить на него; процесс не убивать.

## Шаг 2. Работать через pagectl

```powershell
pagectl tabs                    # список вкладок, * — текущая
pagectl use 2                   # переключиться: вкладка станет активной в окне
pagectl open <url>              # перейти в текущей вкладке
pagectl newtab <url>            # новая вкладка

pagectl snapshot                # что на странице, со ссылками [eN]
pagectl find '<css|xpath|текст>'
pagectl text                    # видимый текст
pagectl shot "$env:TEMP\page.png"

pagectl click e31
pagectl type e29 'текст'
```

Рабочий цикл: `snapshot` → действие по ссылке → `snapshot` заново. Ссылки `[eN]`
живут до перехода на другую страницу; после навигации снимок протухает, и
`click` скажет об этом прямо.

Если элемента нет в снимке — искать `find`, он понимает CSS, XPath и простой
текст одним вызовом.

После перезапуска Helium вкладки восстановлены, но не загружены, пока их не
активировать — как и у пользователя, который по ним ещё не щёлкал. На такой
вкладке любая команда сразу отвечает «не отвечает: выгружена…»; `pagectl use <n>`
активирует её и дожидается загрузки.

Ввод проходит только в видимую вкладку: окно не свёрнуто, вкладка активна.
Если пользователь сам переключил вкладку или свернул окно, `click` и `type`
через 30 с (`PAGECTL_TIMEOUT`) падают с `context deadline exceeded` и
подсказкой. Тогда `pagectl use <n>` и повторить; свёрнутое окно — попросить
развернуть.

`shot` без пути кладёт `shot.png` в текущий каталог — давать полный путь.

## Шаг 3. Закрыть порт

Когда работа закончена, предложить пользователю вернуть браузер в обычный режим:
тот же перезапуск, что в шаге 1, но без порта:

```powershell
$helium = "$env:LOCALAPPDATA\imput\Helium\Application\chrome.exe"
function Get-Helium { Get-Process chrome -ErrorAction SilentlyContinue | Where-Object Path -eq $helium }
Get-CimInstance Win32_Process -Filter "Name='chrome.exe'" |
  Where-Object { $_.ExecutablePath -eq $helium -and $_.CommandLine -notmatch '--type=' } |
  ForEach-Object { taskkill /PID $_.ProcessId | Out-Null }
$t = (Get-Date).AddSeconds(20); while ((Get-Helium) -and (Get-Date) -lt $t) { Start-Sleep -Milliseconds 300 }
if (Get-Helium) { throw 'Helium не закрылся за 20 с' }
Start-Process $helium -ArgumentList '--restore-last-session'
```

Порт 9222 не требует авторизации: пока он открыт, любой локальный процесс
получает полный доступ ко всем залогиненным сессиям пользователя.

## Опасные места

**Не гасить Helium через `Stop-Process`, `taskkill /F` или `taskkill /IM chrome.exe`.**
Принудительное завершение не даёт браузеру штатно записать сессию, а по имени
`chrome.exe` заодно закроется Google Chrome. Только `taskkill /PID` главного
процесса и без `/F`, как в шаге 1.

**`rodkit stop` не запускать.** Он гасит только браузер, поднятый
`rodkit start`; на живой сессии он откажется, но звать его незачем.

**`chrome://` через CDP не открывается** — `net::ERR_CONNECTION_CLOSED`. Флаги,
настройки и расширения пользователь открывает руками.

**Кнопка Relaunch на странице флагов** на macOS поднимала браузер без
`--remote-debugging-port`. После смены флагов перезапускать командами из шага 1
и проверять `pagectl tabs`.

**Переменные окружения** `Start-Process` передаёт Helium из текущей оболочки
(`$env:TZ = 'Europe/Helsinki'` перед запуском). Учитывает ли Helium под Windows
именно `TZ` — не проверялось.

## Когда всё-таки rodkit

`rodkit` умеет то же плюс `eval`. Брать его только для диагностики самой
страницы, когда без выполнения JS ответа не получить. Файл сессии у него общий
с pagectl; если rodkit пишет, что браузер не отвечает, — сначала
`pagectl tabs`, он обновит адрес.

Цена: rodkit ищет элементы JS-хелперами rod, те зовут `document.querySelector`,
а сайт может его подменить и снять стек — в стеке видно `functions.element`.
`pagectl` ходит доменами DOM/Page/Input, мимо JS-окружения страницы, и на
перехвате `querySelector` даёт ноль вызовов.

`rodkit eval` оборачивает аргумент в `() => (...)`, поэтому нужен expression:
многострочный код писать как `(()=>{ ... })()`. Строки внутри JS — в одинарных
кавычках (см. PowerShell 5.1 выше):
`rodkit eval "document.querySelector('h1').textContent"`.

`$env:RODKIT_CONSOLE = '1'` включает сбор консоли, а вместе с ним домен Runtime —
после этого детекторы ставят `isAutomatedWithCDP: true`. Только для отладки, и
не в том прогоне, результат которого показываешь пользователю.

## Почему так, а не антидетект-браузер

Профиль настоящий: история, куки, шрифты, видеокарта — ничего не подделывается,
поэтому и рассыпаться нечему. Camoufox и прочие подставляют сгенерированную
личность, и там нет логинов пользователя. Библиотека rod при подключении к
вкладке по умолчанию включала бы эмуляцию устройства — чужой User-Agent, язык
`en` и вьюпорт 1280×800 на время команды; в обоих инструментах это отключено и
закреплено тестом, а страницей-пробником проверено, что `navigator.userAgent`,
язык и размер окна во время `click` не меняются.

Ввод уже человекоподобный: курсор идёт дугой Безье, клик попадает мимо центра
элемента, текст набирается посимвольно настоящими key-событиями с Shift где
надо, кириллица — через раскладку ЙЦУКЕН. Механика ввода общая у обоих
инструментов (пакет `human`), и через rodkit она даёт `isBot: false` на
`deviceandbrowserinfo.com/are_you_a_bot_interactions` (замер на macOS). Под
Windows механика та же; на тестовой странице события приходят с
`isTrusted: true`, клик мимо центра, заглавные и `@` — с Shift.
