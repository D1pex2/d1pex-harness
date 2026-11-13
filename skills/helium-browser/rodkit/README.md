# rodkit

Обёртка над [go-rod](https://github.com/go-rod/rod): браузерные проверки для тестов и
тот же браузер в интерактивном режиме — по шагам из оболочки.

## Пакеты

* `browser` — запуск Chromium, поиск по CSS и по роли, ожидающие проверки (`ExpectVisible`,
  `ExpectRoleEnabled`, `ExpectGone`), дамп вкладки при падении.
* `wait` — опрос до дедлайна: `Until`, `UntilEvery`, `Consistently`, `Never`.

В тестах:

```go
b := browser.Launch(t, "http://frontend.test", true)
tab := b.Open(t, "/booking/42")
tab.ExpectRoleEnabled(ctx, "кнопка оплаты", "button", regexp.MustCompile(`^Оплатить`)).Click(ctx)
```

## Интерактивный режим

```
go install ./cmd/rodkit

rodkit start --base http://frontend.test &   # держать в фоне: браузер + консоль страниц
rodkit open /booking/42
rodkit state                                 # что вообще есть на странице
rodkit fill '#email' guest@example.com
rodkit click 'role/button/^Оплатить'
rodkit shot /tmp/после-оплаты.png
rodkit stop
```

Сессия живёт в процессе `rodkit start`, остальные команды подключаются к ней по CDP —
вкладка переживает команду. `rodkit` без аргументов печатает полный список.

Цель поиска: CSS-селектор, `role/<роль>/<регулярка>` или `text/<регулярка>`.
Таймаут ожиданий — `RODKIT_TIMEOUT` (10s), путь к Chromium — `E2E_CHROMIUM_BIN`.
Без него берётся кеш Playwright, затем установленный Chrome или Edge; rod качает
свой Chromium, только если не нашлось ничего.

## Живой браузер: pagectl

`pagectl` работает с уже открытым браузером пользователя и не выполняет JS на
странице. Браузер должен слушать порт отладки:

```
pagectl connect          # 127.0.0.1:9222; можно дать порт или адрес
pagectl tabs
```

Адрес CDP меняется с каждым запуском браузера — после перезапуска оба
инструмента сами спрашивают порт заново и переписывают `~/.rodkit/session.json`.
Без файла сессии pagectl идёт на 127.0.0.1:9222. `rodkit stop` такую сессию не
трогает: гасит только браузер, поднятый `rodkit start`.

Вкладки, восстановленные после перезапуска, не загружены до первой активации,
и подключение к ним виснет (rod шлёт `Page.enable`, отвечать некому). Поэтому
оба инструмента подключаются только к текущей вкладке, перед этим проверяют
пробником, что она отвечает, а `use <n>` активирует вкладку и ждёт загрузки.

## Windows

Всё то же, с поправками на оболочку:

```powershell
go install ./cmd/...     # в %USERPROFILE%\go\bin, он в PATH

Start-Process rodkit -ArgumentList 'start', '--base', 'http://frontend.test' `
  -WindowStyle Hidden -RedirectStandardOutput "$env:TEMP\rodkit-console.log"
rodkit open /booking/42
rodkit shot "$env:TEMP\после-оплаты.png"
rodkit stop
```

`rodkit stop` в Windows не шлёт сигнал — их там нет, — а взводит именованное
событие, которое ждёт `rodkit start`: сессия закрывается так же штатно, как по
Ctrl-C. Из Git Bash работает и `rodkit start &`.

* Git Bash переписывает аргументы, начинающиеся с `/`: `rodkit open /booking/42`
  уедет как `C:/Program Files/Git/booking/42`, XPath `//button` — как `/button`.
  Лечится `MSYS_NO_PATHCONV=1`.
* Windows PowerShell 5.1 выбрасывает `"` внутри аргументов:
  `'//a[@id="x"]'` доедет как `//a[@id=x]`. Внутри селекторов и JS — одинарные
  кавычки.
* Лог `rodkit start` в UTF-8 без BOM; PowerShell 5.1 читает его как
  `Get-Content -Encoding UTF8`.
* Окно, перекрытое другими, Chrome под Windows считает невидимым и перестаёт
  рисовать; движение мыши подтверждается только на кадре, и ввод повисает.
  Живой браузер поднимать с `--disable-backgrounding-occluded-windows` (rod
  ставит этот флаг своим браузерам сам). pagectl не висит вечно: срок на
  команду — `PAGECTL_TIMEOUT` (30s).
