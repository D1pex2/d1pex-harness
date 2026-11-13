// Package browser — тонкая обёртка над go-rod: запуск Chromium и проверки,
// которые сами опрашивают до дедлайна.
//
// Playwright давал авто-повтор через expect(); здесь ту же роль играет
// wait.Until, на котором и так стоит весь остальной набор.
//
// Ролевой поиск (get_by_role у Playwright) приближается сопоставлением текста
// элемента: у фронта кнопки — обычные <button> с подписью, поэтому вычисленное
// имя доступности совпадает с текстом. Если появятся aria-label без текста,
// сюда придётся добавить чтение атрибута.
package browser

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"rodkit/cdp"
	"rodkit/wait"
)

// Бинари берём из кеша Playwright: они уже скачаны, второй браузер не нужен.
//
// Порядок не косметический. chrome-headless-shell — урезанная сборка без UI,
// её же Playwright запускает в headless. На ней старт 60ms против 170ms у
// полного Chrome, переход 122ms против 232ms. Полный нужен только когда
// смотришь глазами (headless=false).
//
// Под Windows ревизия в пути — шаблон: кеш Playwright там ни к какой сборке не
// привязан, берётся самая свежая из скачанных. Раскладка каталогов у
// Playwright менялась, поэтому по два варианта на каждый бинарь.
var headlessCandidates = map[string][]string{
	"darwin": {
		"~/Library/Caches/ms-playwright/chromium_headless_shell-1217/chrome-headless-shell-mac-arm64/chrome-headless-shell",
	},
	"windows": {
		`%LOCALAPPDATA%\ms-playwright\chromium_headless_shell-[0-9]*\chrome-headless-shell-win64\chrome-headless-shell.exe`,
		`%LOCALAPPDATA%\ms-playwright\chromium_headless_shell-[0-9]*\chrome-win\headless_shell.exe`,
	},
}[runtime.GOOS]

var headedCandidates = map[string][]string{
	"darwin": {
		"~/Library/Caches/ms-playwright/chromium-1217/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	},
	"windows": {
		`%LOCALAPPDATA%\ms-playwright\chromium-[0-9]*\chrome-win64\chrome.exe`,
		`%LOCALAPPDATA%\ms-playwright\chromium-[0-9]*\chrome-win\chrome.exe`,
	},
}[runtime.GOOS]

// Launch поднимает браузер на время теста.
// Путь к бинарю можно задать через E2E_CHROMIUM_BIN.
func Launch(t wait.TB, baseURL string, headless bool) *Browser {
	t.Helper()

	l := launcher.New().Headless(headless).
		// OrbStack слушает порты и на IPv4, и на IPv6; по IPv6 соединение с
		// traefik виснет и Chrome отдаёт ERR_EMPTY_RESPONSE. Тот же сюрприз
		// уже ловили с gRPC-клиентом Temporal. Прибиваем имена стенда к IPv4.
		Set("host-resolver-rules", "MAP frontend.test 127.0.0.1, MAP *.test 127.0.0.1").
		// Раскладка фронта переключается на 900px (--breakpoint-tablet), и от неё
		// зависит порядок шагов checkout: на десктопе форма гостя видна сразу, на
		// мобильной — только после кнопки шага 1. Без явного размера бралcя дефолт
		// окна Chrome, то есть прогон зависел от версии и машины.
		Set("window-size", "1280,800")
	if bin := findChromium(headless); bin != "" {
		l = l.Bin(bin)
	}

	url, err := l.Launch()
	if err != nil {
		t.Fatalf("не удалось запустить браузер: %v", err)
	}

	b := rod.New().ControlURL(url)
	if err := b.Connect(); err != nil {
		t.Fatalf("не удалось подключиться к браузеру: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	return &Browser{rod: b, baseURL: baseURL, controlURL: url}
}

func findChromium(headless bool) string {
	if bin := os.Getenv("E2E_CHROMIUM_BIN"); bin != "" {
		return bin
	}
	candidates := headedCandidates
	if headless {
		candidates = append(append([]string{}, headlessCandidates...), headedCandidates...)
	}
	home, _ := os.UserHomeDir()
	for _, path := range candidates {
		if len(path) > 1 && path[:2] == "~/" {
			path = filepath.Join(home, path[2:])
		}
		path = winEnv.ReplaceAllStringFunc(path, func(v string) string { return os.Getenv(v[1 : len(v)-1]) })
		matches, _ := filepath.Glob(path)
		// Номер ревизии растёт, так что последняя по алфавиту — самая свежая.
		for i := len(matches) - 1; i >= 0; i-- {
			if st, err := os.Stat(matches[i]); err == nil && !st.IsDir() {
				return matches[i]
			}
		}
	}
	// Системный Chrome или Edge. Без этого шага rod при пустом Bin не ищет
	// установленный браузер, а сразу качает свой Chromium на ~150 МБ.
	if bin, ok := launcher.LookPath(); ok {
		return bin
	}
	return "" // rod скачает свой
}

// winEnv — переменная окружения в записи Windows: %LOCALAPPDATA%.
var winEnv = regexp.MustCompile(`%\w+%`)

type Browser struct {
	rod        *rod.Browser
	baseURL    string
	controlURL string

	wrapMu  sync.Mutex
	wrapped map[proto.TargetTargetID]*Tab
}

// ControlURL — адрес CDP запущенного браузера. Нужен интерактивному режиму:
// команды выполняются отдельными процессами и подключаются к нему через Attach.
func (b *Browser) ControlURL() string { return b.controlURL }

func (b *Browser) Close() { _ = b.rod.Close() }

// Attach подключается к уже работающему браузеру вместо запуска своего.
//
// Без NoDefaultDevice rod на каждой вкладке включил бы эмуляцию устройства —
// чужой User-Agent, язык и вьюпорт на время команды. Для живого браузера
// пользователя это недопустимо, подробнее в cdp.Attach. Своему браузеру из
// Launch эмуляция не мешает: там и профиль временный, и личности нет.
func Attach(t wait.TB, controlURL, baseURL string) *Browser {
	t.Helper()
	b := rod.New().NoDefaultDevice().ControlURL(controlURL)
	if err := b.Connect(); err != nil {
		t.Fatalf("не удалось подключиться к браузеру по %s: %v", controlURL, err)
	}
	return &Browser{rod: b, baseURL: baseURL, controlURL: controlURL}
}

// Tabs — все открытые вкладки браузера с подключением к каждой. Тесту не нужны:
// он держит ссылки на те, что открыл сам. Нужны сессии rodkit start, которая
// читает консоль всех вкладок своего браузера. К живому браузеру пользователя
// так ходить нельзя: подключение к выгруженной вкладке виснет, см. Tab.
func (b *Browser) Tabs(t wait.TB) []*Tab {
	pages, err := b.rod.Pages()
	if err != nil {
		t.Fatalf("список вкладок: %v", err)
	}
	tabs := make([]*Tab, 0, len(pages))
	for _, page := range pages {
		tabs = append(tabs, b.newTab(t, page))
	}
	return tabs
}

func (tab *Tab) TargetID() string { return string(tab.rod.TargetID) }

// Target — вкладка, как её перечисляет браузер: без подключения к ней.
type Target struct {
	ID  string
	URL string
}

// Targets — открытые вкладки без подключения. Интерактивный режим выбирает по
// этому списку одну и подключается только к ней через Tab.
func (b *Browser) Targets(t wait.TB) []Target {
	t.Helper()
	res, err := proto.TargetGetTargets{}.Call(b.rod)
	if err != nil {
		t.Fatalf("список вкладок: %v", err)
	}
	var out []Target
	for _, info := range res.TargetInfos {
		if info.Type == "page" {
			out = append(out, Target{ID: string(info.TargetID), URL: info.URL})
		}
	}
	return out
}

// Tab подключается к одной вкладке по TargetID.
//
// Отдельно от Tabs, потому что к выгруженной вкладке подключение виснет: rod
// шлёт ей Page.enable, а рендерера у восстановленной после перезапуска вкладки
// нет до первой активации. Живой браузер после перезапуска почти целиком из
// таких, поэтому сначала пробник cdp.Loaded.
func (b *Browser) Tab(t wait.TB, id string) *Tab {
	t.Helper()
	ok, err := cdp.Loaded(b.rod, proto.TargetTargetID(id))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !ok {
		t.Fatalf("вкладка %s не отвечает: выгружена после восстановления сессии или занята — rodkit use <n> активирует и загрузит её", id)
	}
	page, err := b.rod.PageFromTarget(proto.TargetTargetID(id))
	if err != nil {
		t.Fatalf("подключение к вкладке %s: %v", id, err)
	}
	return b.newTab(t, page)
}

// Activate показывает вкладку — как щелчок по ней в полосе вкладок — и
// дожидается её рендерера: восстановленная вкладка загружается только с
// активации, а в фоновую не проходит ввод.
func (b *Browser) Activate(ctx context.Context, t wait.TB, id string) {
	t.Helper()
	if err := (proto.TargetActivateTarget{TargetID: proto.TargetTargetID(id)}).Call(b.rod); err != nil {
		t.Fatalf("активация вкладки %s: %v", id, err)
	}
	wait.UntilEvery(ctx, t, 200*time.Millisecond, func() (string, bool) {
		ok, err := cdp.Loaded(b.rod, proto.TargetTargetID(id))
		if err != nil {
			return err.Error(), false
		}
		return "рендерер ещё не отвечает", ok
	}, "вкладка загрузилась после активации")
}

// Open открывает путь относительно базового адреса стенда.
func (b *Browser) Open(t wait.TB, path string) *Tab {
	t.Helper()

	tab, err := b.rod.Page(proto.TargetCreateTarget{URL: b.baseURL + path})
	if err != nil {
		t.Fatalf("не удалось открыть %s: %v", path, err)
	}
	_ = tab.Timeout(openTimeout).WaitLoad()
	return b.newTab(t, tab)
}

// WaitNewTab выполняет trigger и ждёт вкладку, которую открыла эта — так уезжают на платёжную
// форму банка. Метод вкладки, а не браузера: новая вкладка всегда чьё-то
// следствие, и тесту незачем держать ссылку на браузер ради одного ожидания.
func (tab *Tab) WaitNewTab(ctx context.Context, trigger func()) *Tab {
	t, b := tab.t, tab.browser
	t.Helper()

	before, _ := b.rod.Pages()
	seen := map[proto.TargetTargetID]bool{}
	for _, tab := range before {
		seen[tab.TargetID] = true
	}
	trigger()

	fresh := wait.UntilEvery(ctx, t, pollInterval, func() (*rod.Page, bool) {
		pages, err := b.rod.Pages()
		if err != nil {
			return nil, false
		}
		for _, tab := range pages {
			if !seen[tab.TargetID] {
				return tab, true
			}
		}
		return nil, false
	}, "банк открыл новую вкладку")

	_ = fresh.Timeout(openTimeout).WaitLoad()
	return b.newTab(t, fresh)
}

type Tab struct {
	rod     *rod.Page
	browser *Browser
	t       wait.TB

	consoleMu sync.Mutex
	console   []string
}

// Живые вкладки прогона: по ним снимается дамп из мест, где ссылки на страницу
// нет — затянувшееся ожидание и Ctrl-C.
var (
	liveMu   sync.Mutex
	liveTabs []*Tab
	hookOnce sync.Once
)

// newTab — единственная точка рождения вкладки: цепляет сбор консоли и дамп
// состояния при падении. Cleanup'ы идут в обратном порядке, поэтому дамп успеет
// снять экран до того, как Launch закроет браузер.
func (b *Browser) newTab(t wait.TB, src *rod.Page) *Tab {
	// Одна вкладка — одна обёртка: интерактивный режим перечисляет вкладки в
	// цикле, и без этого каждый обход вешал бы на страницу ещё один сборщик
	// консоли, а сообщения множились бы по числу обходов.
	b.wrapMu.Lock()
	if known, ok := b.wrapped[src.TargetID]; ok {
		b.wrapMu.Unlock()
		return known
	}
	tab := &Tab{rod: src, browser: b, t: t}
	if b.wrapped == nil {
		b.wrapped = map[proto.TargetTargetID]*Tab{}
	}
	b.wrapped[src.TargetID] = tab
	b.wrapMu.Unlock()

	tab.watchConsole()
	installDumpHooks()

	liveMu.Lock()
	liveTabs = append(liveTabs, tab)
	liveMu.Unlock()

	t.Cleanup(func() {
		if t.Failed() {
			tab.Dump("тест упал")
		}
		b.wrapMu.Lock()
		delete(b.wrapped, src.TargetID)
		b.wrapMu.Unlock()
		liveMu.Lock()
		defer liveMu.Unlock()
		liveTabs = slices.DeleteFunc(liveTabs, func(x *Tab) bool { return x == tab })
	})
	return tab
}

// InterruptHandled выставляет процесс, который сам ловит Ctrl-C и завершается
// штатно, — rodkit start. Тогда дамп сигнал дальше не передаёт: в тесте без
// этого прогон не завершился бы вовсе, а здесь досрочный выход оборвал бы
// закрытие сессии, и на диске остался бы файл с мёртвым pid.
var InterruptHandled bool

// installDumpHooks подписывает дамп на два случая, где иначе состояние теряется:
// ожидание, которое ещё не дошло до дедлайна, и прогон, прерванный руками.
func installDumpHooks() {
	hookOnce.Do(func() {
		wait.OnSlow = func(what string) { dumpLive("ожидание: " + what) }

		// Снимаем дамп и отдаём сигнал дальше штатному обработчику. Свой os.Exit
		// тут гасил прогон молча: ни строки результата, ни кода выхода — а под
		// оболочкой, которая шлёт SIGINT при уходе в фон, это выглядело как
		// зависание на ровном месте.
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		go func() {
			<-interrupt
			dumpLive("прогон прерван")
			signal.Stop(interrupt)
			if !InterruptHandled {
				reraiseInterrupt()
			}
		}()
	})
}

func dumpLive(reason string) {
	liveMu.Lock()
	pages := slices.Clone(liveTabs)
	liveMu.Unlock()
	for _, tab := range pages {
		tab.Dump(reason)
	}
}

// Goto переходит по пути относительно базового адреса.
func (tab *Tab) Goto(path string) {
	tab.t.Helper()
	if err := tab.rod.Navigate(tab.browser.baseURL + path); err != nil {
		tab.t.Fatalf("переход на %s: %v", path, err)
	}
	_ = tab.rod.Timeout(openTimeout).WaitLoad()
}

func (tab *Tab) Reload() {
	tab.t.Helper()
	if err := tab.rod.Reload(); err != nil {
		tab.t.Fatalf("перезагрузка: %v", err)
	}
	_ = tab.rod.WaitLoad()
}

// SetContent подменяет разметку страницы. Нужен только тестам самой обёртки:
// проверять поиск и видимость удобнее на предсказуемом DOM, а не на вёрстке
// приложения, которая меняется.
func (tab *Tab) SetContent(html string) {
	tab.t.Helper()
	if _, err := tab.rod.Eval(`html => document.body.innerHTML = html`, html); err != nil {
		tab.t.Fatalf("подмена разметки: %v", err)
	}
}

func (tab *Tab) URL() string {
	info, err := tab.rod.Info()
	if err != nil {
		return ""
	}
	return info.URL
}

func (tab *Tab) Close() { _ = tab.rod.Close() }

// Text — видимый текст страницы. Нужен проверкам, которые смотрят не на один
// элемент, а на то, что гость читает целиком: суммы в чеке и на checkout
// разложены по разным блокам, и искать их поэлементно значит завязываться на
// вёрстку, которая меняется чаще самих сумм.
func (tab *Tab) Text(ctx context.Context) string {
	probe := tab.probe(ctx)
	defer probe.CancelTimeout()
	text, err := probe.Eval(`() => document.body.innerText`)
	if err != nil {
		return ""
	}
	return text.Value.Str()
}

// ---------- поиск ----------

// Проба и такт опроса.
//
// rod по умолчанию ждёт появления элемента БЕСКОНЕЧНО: tab.Element блокируется,
// пока элемент не возникнет. Нам это ломает всю схему — опросом управляет
// wait.UntilEvery, а поиск обязан быстро отвечать «пока нет».
//
// Раньше это решалось коротким Timeout, но Timeout не отключает внутренний
// ретрай rod (utils.Retry + DefaultSleeper, backoff 100ms→1s) — он лишь ставит
// ему дедлайн. Отсутствующий элемент честно отжирал весь срок, и промах стоил
// lookupTimeout + такт опроса. Вместо этого берём rod'овские Has/HasR: они
// сами ходят с NotFoundSleeper, то есть делают ровно один запрос в браузер.
//
// lookupTimeout теперь не регулятор, а страховка от зависшего CDP-вызова,
// поэтому он щедрый: в норме не срабатывает никогда. Коротким его делать
// опасно — он рубит не ожидание, а сам eval, и на тяжёлом DOM превращается
// в ложное «элемента нет».
const lookupTimeout = 2 * time.Second

// pollInterval — такт опроса DOM. Страница перерисовывается за миллисекунды,
// ждать её бэкендным тактом wait.Interval незачем.
const pollInterval = 50 * time.Millisecond

// openTimeout — потолок на открытие вкладки: страховка от зависшего CDP-вызова.
const openTimeout = 30 * time.Second

func (tab *Tab) probe(ctx context.Context) *rod.Page {
	return tab.rod.Context(ctx).Timeout(lookupTimeout)
}

func (e *Element) probe(ctx context.Context) *rod.Element {
	return e.rod.Context(ctx).Timeout(lookupTimeout)
}

// Element — первый элемент по CSS-селектору. ok=false, если его ещё нет.
func (tab *Tab) Element(ctx context.Context, css string) (*Element, bool) {
	probe := tab.probe(ctx)
	defer probe.CancelTimeout()
	ok, el, err := probe.Has(css)
	if err != nil || !ok || el == nil {
		return nil, false
	}
	return &Element{rod: el.Context(ctx), t: tab.t}, true
}

// ByRole — элемент роли с подписью по регулярке. Роль отображается в набор
// тегов: button → button и [role=button], heading → h1..h6.
//
// name == nil — любой элемент этой роли: у чекбокса текста нет, и поиск по
// тексту его не находит.
// Совпадений может быть несколько: адаптивный фронт держит в DOM и мобильный,
// и десктопный вариант одной кнопки, лишний спрятан CSS. Гость видит ровно
// один, поэтому видимый в приоритете; скрытый отдаём, только если видимого нет
// вовсе — так ожидания видят элемент и пишут в лог, а не молчат про «не найден».
//
// Текст берём из textContent, а не innerText: у скрытого элемента innerText
// пустой, и совпадение по подписи для него не сработало бы вовсе.
func (tab *Tab) ByRole(ctx context.Context, role string, name *regexp.Regexp) (*Element, bool) {
	if name == nil {
		return tab.Element(ctx, roleSelector(role))
	}
	var hidden *Element
	for _, el := range tab.Elements(ctx, roleSelector(role)) {
		text, err := el.rod.Context(ctx).Property("textContent")
		if err != nil || !name.MatchString(normalizeText(strings.TrimSpace(text.String()))) {
			continue
		}
		if el.Visible(ctx) {
			return el, true
		}
		if hidden == nil {
			hidden = el
		}
	}
	return hidden, hidden != nil
}

// Elements — все элементы по CSS-селектору. Нужны там, где тест берёт
// последний из списка (например согласие — последний чекбокс формы).
func (tab *Tab) Elements(ctx context.Context, css string) []*Element {
	probe := tab.probe(ctx)
	defer probe.CancelTimeout()
	els, err := probe.Elements(css)
	if err != nil {
		return nil
	}
	out := make([]*Element, 0, len(els))
	for _, el := range els {
		out = append(out, &Element{rod: el.Context(ctx), t: tab.t})
	}
	return out
}

// ByText — первый элемент, чей текст совпал с регуляркой.
func (tab *Tab) ByText(ctx context.Context, re *regexp.Regexp) (*Element, bool) {
	probe := tab.probe(ctx)
	defer probe.CancelTimeout()
	ok, el, err := probe.HasR("*", jsRegex(re))
	if err != nil || !ok || el == nil {
		return nil, false
	}
	return &Element{rod: el.Context(ctx), t: tab.t}, true
}

// jsRegex переводит регулярку Go в литерал JS.
//
// rod подставляет строку прямо в new RegExp, а встроенных флагов вида (?i) в
// JS нет — Chrome отвечает «Invalid group». Питоновский набор держит re.I почти
// во всех текстовых проверках, так что без перевода отвалилась бы каждая.
func jsRegex(re *regexp.Regexp) string {
	pattern := re.String()
	flags := ""
	for {
		switch {
		case strings.HasPrefix(pattern, "(?i)"):
			flags, pattern = flags+"i", pattern[4:]
		case strings.HasPrefix(pattern, "(?s)"):
			flags, pattern = flags+"s", pattern[4:]
		case strings.HasPrefix(pattern, "(?m)"):
			flags, pattern = flags+"m", pattern[4:]
		default:
			return "/" + strings.ReplaceAll(pattern, "/", `\/`) + "/" + flags
		}
	}
}

func roleSelector(role string) string {
	switch role {
	case "button":
		return "button, [role=button]"
	case "heading":
		return "h1, h2, h3, h4, h5, h6, [role=heading]"
	case "checkbox":
		return "input[type=checkbox], [role=checkbox]"
	default:
		return "[role=" + role + "]"
	}
}

type Element struct {
	rod *rod.Element
	t   wait.TB
}

// Element — вложенный поиск внутри элемента (аналог receipt.locator(...)).
func (e *Element) Element(ctx context.Context, css string) (*Element, bool) {
	probe := e.probe(ctx)
	defer probe.CancelTimeout()
	ok, el, err := probe.Has(css)
	if err != nil || !ok || el == nil {
		return nil, false
	}
	return &Element{rod: el.Context(ctx), t: e.t}, true
}

// Тот же приоритет видимого, что и у Tab.ByRole — причина та же.
func (e *Element) ByRole(ctx context.Context, role string, name *regexp.Regexp) (*Element, bool) {
	if name == nil {
		return e.Element(ctx, roleSelector(role))
	}
	probe := e.probe(ctx)
	defer probe.CancelTimeout()
	els, err := probe.Elements(roleSelector(role))
	if err != nil {
		return nil, false
	}
	var hidden *Element
	for _, el := range els {
		text, err := el.Context(ctx).Property("textContent")
		if err != nil || !name.MatchString(normalizeText(strings.TrimSpace(text.String()))) {
			continue
		}
		found := &Element{rod: el.Context(ctx), t: e.t}
		if found.Visible(ctx) {
			return found, true
		}
		if hidden == nil {
			hidden = found
		}
	}
	return hidden, hidden != nil
}

func (e *Element) ByText(ctx context.Context, re *regexp.Regexp) (*Element, bool) {
	probe := e.probe(ctx)
	defer probe.CancelTimeout()
	ok, el, err := probe.HasR("*", jsRegex(re))
	if err != nil || !ok || el == nil {
		return nil, false
	}
	return &Element{rod: el.Context(ctx), t: e.t}, true
}

func normalizeText(s string) string {
	return strings.NewReplacer("\u00a0", " ", "\u202f", " ").Replace(s)
}

// Visible — посчитан ли элемент видимым. Именно этот вопрос требует настоящего
// движка рендеринга: 26 проверок набора спрашивают только его.
func (e *Element) Visible(ctx context.Context) bool {
	v, err := e.rod.Context(ctx).Visible()
	return err == nil && v
}

// Checked — состояние переключателя. Роль checkbox носит не только <input>:
// UI-кит рисует её как <button role="checkbox" aria-checked>, и в DOM эта кнопка
// идёт раньше скрытого input. У кнопки свойства checked нет вовсе, поэтому
// сначала спрашиваем aria-checked, а свойство остаётся для нативных полей.
func (e *Element) Checked(ctx context.Context) bool {
	if aria, err := e.rod.Context(ctx).Attribute("aria-checked"); err == nil && aria != nil {
		return *aria == "true"
	}
	value, err := e.rod.Context(ctx).Property("checked")
	return err == nil && value.Bool()
}

// Enabled — не выставлен ли disabled.
func (e *Element) Enabled(ctx context.Context) bool {
	attr, err := e.rod.Context(ctx).Attribute("disabled")
	return err == nil && attr == nil
}

func (e *Element) Text(ctx context.Context) string {
	text, err := e.rod.Context(ctx).Text()
	if err != nil {
		return ""
	}
	return text
}

func (e *Element) Fill(ctx context.Context, value string) {
	e.t.Helper()
	el := e.rod.Context(ctx)
	if err := humanType(ctx, el, value); err != nil {
		e.t.Fatalf("ввод %q: %v", value, err)
	}
}

func (e *Element) Click(ctx context.Context) {
	e.t.Helper()
	if err := humanClick(ctx, e.rod.Context(ctx)); err != nil {
		e.t.Fatalf("клик: %v", err)
	}
}

func (e *Element) SetFiles(ctx context.Context, paths ...string) {
	e.t.Helper()
	if err := e.rod.Context(ctx).SetFiles(paths); err != nil {
		e.t.Fatalf("загрузка файлов %v: %v", paths, err)
	}
}

// ---------- ожидающие проверки ----------

// ExpectVisible ждёт, пока элемент появится и станет видимым.
func (tab *Tab) ExpectVisible(ctx context.Context, what, css string) *Element {
	tab.t.Helper()
	return wait.UntilEvery(ctx, tab.t, pollInterval, func() (*Element, bool) {
		el, ok := tab.Element(ctx, css)
		return el, ok && el.Visible(ctx)
	}, what+" виден")
}

// ExpectRoleVisible ждёт видимый элемент роли с подписью.
func (tab *Tab) ExpectRoleVisible(ctx context.Context, what, role string, name *regexp.Regexp) *Element {
	tab.t.Helper()
	return wait.UntilEvery(ctx, tab.t, pollInterval, func() (*Element, bool) {
		el, ok := tab.ByRole(ctx, role, name)
		return el, ok && el.Visible(ctx)
	}, what+" виден")
}

// ExpectRoleEnabled ждёт активный элемент роли с подписью.
func (tab *Tab) ExpectRoleEnabled(ctx context.Context, what, role string, name *regexp.Regexp) *Element {
	tab.t.Helper()
	return wait.UntilEvery(ctx, tab.t, pollInterval, func() (*Element, bool) {
		el, ok := tab.ByRole(ctx, role, name)
		return el, ok && el.Visible(ctx) && el.Enabled(ctx)
	}, what+" активен")
}

// ExpectAttached ждёт появления элемента в DOM, видимость не проверяя.
func (tab *Tab) ExpectAttached(ctx context.Context, what, css string) *Element {
	tab.t.Helper()
	return wait.UntilEvery(ctx, tab.t, pollInterval, func() (*Element, bool) {
		return tab.Element(ctx, css)
	}, what+" есть в DOM")
}

// ExpectGone ждёт, пока элемента не станет видно.
//
// Единственная проверка набора, которая ЗАВЕРШАЕТСЯ по «не нашли». У всех
// остальных ложный промах безвреден — следующая итерация его исправит. Здесь
// он означает зелёный тест, не проверивший ничего: оборванный eval, страница
// в момент перерисовки или гонка с навигацией читаются как «пропало».
// Поэтому пропажу подтверждаем двумя промахами подряд.
func (tab *Tab) ExpectGone(ctx context.Context, what, role string, name *regexp.Regexp) {
	tab.t.Helper()

	misses := 0
	wait.UntilEvery(ctx, tab.t, pollInterval, func() (int, bool) {
		el, ok := tab.ByRole(ctx, role, name)
		if ok && el.Visible(ctx) {
			misses = 0
			return misses, false
		}
		misses++
		return misses, misses >= 2
	}, what+" пропал")
}

// ExpectURL ждёт, пока адрес страницы совпадёт с регуляркой.
func (tab *Tab) ExpectURL(ctx context.Context, re *regexp.Regexp) {
	tab.t.Helper()
	wait.UntilEvery(ctx, tab.t, pollInterval, func() (string, bool) {
		url := tab.URL()
		return url, re.MatchString(url)
	}, fmt.Sprintf("адрес совпал с %s", re))
}

// ExpectTextVisible ждёт видимый элемент с текстом по регулярке.
func (tab *Tab) ExpectTextVisible(ctx context.Context, what string, re *regexp.Regexp) *Element {
	tab.t.Helper()
	return wait.UntilEvery(ctx, tab.t, pollInterval, func() (*Element, bool) {
		el, ok := tab.ByText(ctx, re)
		return el, ok && el.Visible(ctx)
	}, what+" виден")
}

// WaitElement ждёт элемент по произвольному условию — для случаев, где поиск
// идёт внутри уже найденного узла (строка чека, кнопка внутри чека).
func WaitElement(ctx context.Context, t wait.TB, what string, fn func() (*Element, bool)) *Element {
	t.Helper()
	return wait.UntilEvery(ctx, t, pollInterval, fn, what)
}

// WaitValue ждёт произвольное значение — например пока чек наполнится текстом.
func WaitValue[T any](ctx context.Context, t wait.TB, what string, fn func() (T, bool)) T {
	t.Helper()
	return wait.UntilEvery(ctx, t, pollInterval, fn, what)
}

// ClickRoleIn заново находит контейнер и кнопку при каждой попытке: React может
// заменить узел между поиском и кликом, пока кабинет обновляет бронь.
func (tab *Tab) ClickRoleIn(ctx context.Context, what, container, role string, name *regexp.Regexp) {
	tab.t.Helper()
	WaitValue(ctx, tab.t, what+" нажалась", func() (string, bool) {
		root, ok := tab.Element(ctx, container)
		if !ok {
			return "контейнер не найден", false
		}
		el, ok := root.ByRole(ctx, role, name)
		if !ok || !el.Visible(ctx) || !el.Enabled(ctx) {
			return "кнопка не найдена или неактивна", false
		}
		if err := humanClick(ctx, el.rod.Context(ctx)); err != nil {
			return err.Error(), false
		}
		return "", true
	})
}

// Dump складывает состояние вкладки на диск — заменяет трейс-вьюер Playwright.
// Вешается на t.Cleanup каждой вкладки и срабатывает только у упавшего теста:
// ожидания молчат про то, ЧТО было на экране, а разбирать это постфактум не по
// чему. Пишет снимок, разметку, консоль и сводку по элементам.
//
// Значения полей не сохраняем — только факт заполненности: в форму едут почта и
// телефон гостя, незачем раскладывать их по временным файлам.
func (tab *Tab) Dump(reason string) {
	dir := filepath.Join(tab.t.ArtifactDir(), fmt.Sprintf("browser-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	if data, err := tab.rod.Screenshot(true, nil); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "screen.png"), data, 0o644)
	}
	// Через Eval, а не rod.HTML(): тот ходит за нодой документа и на части
	// страниц возвращал ошибку — разметка в дампе молча пропадала, а именно она
	// нужна, когда проверка не нашла ожидаемый текст.
	if html, err := tab.rod.Eval(`() => document.documentElement.outerHTML`); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "tab.html"), []byte(html.Value.Str()), 0o644)
	}
	if text, err := tab.rod.Eval(`() => document.body.innerText`); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "text.txt"), []byte(text.Value.Str()), 0o644)
	}
	if state := tab.State(); state != "" {
		_ = os.WriteFile(filepath.Join(dir, "state.json"), []byte(state), 0o644)
	}

	tab.consoleMu.Lock()
	log := strings.Join(tab.console, "\n")
	tab.consoleMu.Unlock()
	_ = os.WriteFile(filepath.Join(dir, "console.log"), []byte(log), 0o644)

	tab.t.Logf("дамп страницы (%s): %s", reason, dir)
}

// Eval выполняет выражение на странице и отдаёт результат строкой. Тестам не
// нужен — проверка через JS обходит и видимость, и доступность. Нужен разбору
// руками, где иначе пришлось бы лезть в devtools.
func (tab *Tab) Eval(ctx context.Context, js string) string {
	// rod вызывает переданное как функцию, поэтому голое выражение заворачиваем:
	// иначе document.title превращается в document.title.apply(...).
	if trimmed := strings.TrimSpace(js); !strings.HasPrefix(trimmed, "function") &&
		!strings.HasPrefix(trimmed, "async") && !strings.HasPrefix(trimmed, "()") {
		js = "() => (" + js + ")"
	}
	value, err := tab.rod.Context(ctx).Eval(js)
	if err != nil {
		tab.t.Fatalf("выражение %q: %v", js, err)
		return ""
	}
	return value.Value.String()
}

// State — сводка страницы: адрес, размеры и все кнопки с полями. Дамп кладёт её
// файлом, интерактивный режим печатает как есть — по ней видно, за что кликать.
//
// Значения полей не отдаём, только факт заполненности: в форму едут почта и
// телефон гостя.
func (tab *Tab) State() string {
	state, err := tab.rod.Eval(`() => JSON.stringify({
		url: location.href, width: innerWidth, height: innerHeight,
		secureContext: isSecureContext,
		buttons: [...document.querySelectorAll('button, [role=button], [role=checkbox], a[href]')].map(e => ({
			text: (e.textContent || '').trim().slice(0, 60),
			role: e.getAttribute('role') || e.tagName.toLowerCase(),
			disabled: !!e.disabled || e.getAttribute('aria-disabled') === 'true',
			checked: e.getAttribute('aria-checked'),
			visible: !!(e.offsetWidth || e.offsetHeight || e.getClientRects().length),
		})),
		inputs: [...document.querySelectorAll('input, textarea, select')].map(e => ({
			name: e.name || e.id, type: e.type,
			filled: !!e.value, checked: e.checked,
			visible: !!(e.offsetWidth || e.offsetHeight || e.getClientRects().length),
		})),
	}, null, 1)`)
	if err != nil {
		return ""
	}
	return state.Value.Str()
}

// Screenshot кладёт снимок вкладки в файл.
func (tab *Tab) Screenshot(path string) {
	tab.t.Helper()
	data, err := tab.rod.Screenshot(true, nil)
	if err != nil {
		tab.t.Fatalf("снимок: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		tab.t.Fatalf("запись снимка: %v", err)
	}
}

// watchConsole копит сообщения консоли и необработанные исключения вкладки —
// в дампе это единственный способ увидеть, на чём споткнулся фронт.
//
// Выключено по умолчанию: подписка на RuntimeConsoleAPICalled заставляет rod
// послать Runtime.enable, после чего V8 дёргает Error.prepareStackTrace при
// каждом console.log. Антиботы (deviceandbrowserinfo, DataDome) ловят ровно это
// и ставят isAutomatedWithCDP. Включать только когда правда нужна консоль.
func (tab *Tab) watchConsole() {
	if os.Getenv("RODKIT_CONSOLE") != "1" {
		return
	}
	go tab.rod.EachEvent(
		func(e *proto.RuntimeConsoleAPICalled) {
			for _, arg := range e.Args {
				text := arg.Description
				if text == "" {
					text = arg.Value.String()
				}
				tab.addConsole(fmt.Sprintf("console.%s: %s", e.Type, text))
			}
		},
		func(e *proto.RuntimeExceptionThrown) {
			text := e.ExceptionDetails.Text
			if e.ExceptionDetails.Exception != nil {
				text += " | " + e.ExceptionDetails.Exception.Description
			}
			tab.addConsole("EXCEPTION: " + text)
		},
	)()
}

// Console — накопленные сообщения консоли. Копия: интерактивный режим читает их
// из своего цикла, пока сборщик пишет из событий вкладки.
func (tab *Tab) Console() []string {
	tab.consoleMu.Lock()
	defer tab.consoleMu.Unlock()
	return slices.Clone(tab.console)
}

func (tab *Tab) addConsole(line string) {
	tab.consoleMu.Lock()
	defer tab.consoleMu.Unlock()
	tab.console = append(tab.console, line)
}
