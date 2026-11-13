// Команда rodkit — браузер, которым можно рулить по шагам из оболочки.
//
// Сессия живёт в фоновом процессе `rodkit start`: он держит браузер и печатает
// консоль страниц. Остальные команды — отдельные короткие процессы, они
// подключаются к той же сессии по CDP и работают с текущей вкладкой, адрес
// которой лежит в файле сессии. Поэтому шаг «нажал — посмотрел — нажал» не
// требует ни теста, ни единого процесса на весь сценарий.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"rodkit/browser"
	"rodkit/cdp"
	"rodkit/wait"
)

const usage = `rodkit — интерактивный браузер

  rodkit start [--headed] [--base URL]   поднять сессию (держать в фоне)
  rodkit stop                            погасить сессию (живой браузер не трогает)

  rodkit open <url|путь>                 перейти в текущей вкладке
  rodkit newtab <url|путь>               открыть новую вкладку
  rodkit tabs                            список вкладок, * — текущая
  rodkit use <n>                         сделать вкладку текущей и показать её

  rodkit click <цель>                    дождаться и нажать
  rodkit fill <цель> <значение>          дождаться и ввести
  rodkit expect <цель>                   дождаться видимости
  rodkit gone <цель>                     дождаться исчезновения

  rodkit url                             адрес текущей вкладки
  rodkit text [css]                      видимый текст
  rodkit state                           сводка: кнопки и поля страницы
  rodkit shot [файл.png]                 снимок экрана
  rodkit dump                            снимок + разметка + сводка в каталог
  rodkit eval <js>                       выполнить выражение

Цель: CSS-селектор, либо role/<роль>/<регулярка по тексту>, либо text/<регулярка>.
  rodkit click 'role/button/^Оплатить'
  rodkit fill '#email' guest@example.com

Таймаут ожиданий — RODKIT_TIMEOUT (по умолчанию 10s).
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "start":
		start(args)
	case "stop":
		stop()
	case "open":
		goTo(args, false)
	case "newtab":
		goTo(args, true)
	case "tabs":
		listTabs()
	case "use":
		use(args)
	default:
		onTab(os.Args[1], args)
	}
}

// ---------- сессия ----------

type session struct {
	PID     int    `json:"pid"`
	Control string `json:"control_url"`
	Base    string `json:"base_url"`
	Tab     string `json:"tab"`
	// Managed — браузер поднят rodkit start и принадлежит ему. Файл, собранный
	// для живого браузера пользователя (pagectl connect), флага не несёт: pid
	// там — сам браузер, и stop его не тронет.
	Managed bool `json:"managed,omitempty"`
}

func home() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		fatal("не найден домашний каталог: %v", err)
	}
	return filepath.Join(dir, ".rodkit")
}

func sessionPath() string { return filepath.Join(home(), "session.json") }

func load() session {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		fatal("сессия не запущена — сначала rodkit start (или pagectl connect для живого браузера)")
	}
	var s session
	// BOM оставляет PowerShell 5, если файл писали его Out-File -Encoding utf8.
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &s); err != nil {
		fatal("файл сессии испорчен: %v", err)
	}
	return s
}

func (s session) save() {
	if err := os.MkdirAll(home(), 0o755); err != nil {
		fatal("каталог сессии: %v", err)
	}
	data, _ := json.MarshalIndent(s, "", " ")
	if err := os.WriteFile(sessionPath(), data, 0o644); err != nil {
		fatal("запись сессии: %v", err)
	}
}

func start(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	headed := fs.Bool("headed", false, "показать окно браузера")
	base := fs.String("base", "http://frontend.test", "базовый адрес для относительных путей")
	_ = fs.Parse(args)

	if data, err := os.ReadFile(sessionPath()); err == nil {
		var s session
		if json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &s) == nil && alive(s.PID) {
			if !s.Managed {
				fatal("файл сессии смотрит в живой браузер (pid %d) — уберите %s, если нужна своя сессия",
					s.PID, sessionPath())
			}
			fatal("сессия уже работает (pid %d) — rodkit stop, если нужна новая", s.PID)
		}
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	listenStop(stop)

	t := &shellTB{}
	browser.InterruptHandled = true // Ctrl-C обрабатываем сами, ниже
	b := browser.Launch(t, *base, !*headed)
	defer b.Close()

	session{PID: os.Getpid(), Control: b.ControlURL(), Base: *base, Managed: true}.save()
	defer os.Remove(sessionPath())

	fmt.Printf("сессия поднята: pid %d, база %s\n", os.Getpid(), *base)
	go streamConsole(t, b)

	<-stop
	fmt.Println("сессия закрыта")
}

// streamConsole выносит консоль вкладок в вывод фонового процесса: команды
// живут по секунде каждая и увидеть в них ошибку фронта нельзя — к моменту, когда
// сообщение приходит, процесс команды уже вышел.
func streamConsole(t wait.TB, b *browser.Browser) {
	shown := map[string]int{}
	for range time.Tick(time.Second) {
		for _, tab := range b.Tabs(t) {
			log := tab.Console()
			id := tab.TargetID()
			for _, line := range log[min(shown[id], len(log)):] {
				fmt.Printf("[%s] %s\n", id[:6], line)
			}
			shown[id] = len(log)
		}
	}
}

// stop гасит только собственную сессию. Раньше он слал SIGTERM в любой pid из
// файла сессии — а для живого браузера там pid самого браузера, и stop убивал
// его вместе со всеми вкладками пользователя.
func stop() {
	s := load()
	if !s.Managed {
		fatal("сессия смотрит в живой браузер (pid %d), а не поднята rodkit start — гасить его rodkit не будет", s.PID)
	}
	if err := requestStop(s.PID); err != nil {
		os.Remove(sessionPath())
		fatal("процесс сессии %d не отозвался (%v), файл сессии убран", s.PID, err)
	}
	// Ждём, пока start уберёт файл сессии, — сразу следом можно звать rodkit
	// start. По pid ждать нельзя: на unix вышедший, но ещё не подобранный
	// оболочкой процесс по-прежнему виден.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(sessionPath()); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			fatal("сигнал послан pid %d, но за 10s сессия не закрылась", s.PID)
		}
	}
	fmt.Printf("сессия закрыта (pid %d)\n", s.PID)
}

// ---------- вкладки ----------

// attach подключается к сессии. Абсолютный адрес отменяет базовый: в браузер
// банка или почтовый мок ходят по полному URL, мимо стенда. Абсолютный — любой
// со схемой: file:///C:/… и about:blank к стенду тоже не относятся.
func attach(t wait.TB, target string) (*browser.Browser, session) {
	s := load()
	// Адрес живого браузера меняется с каждым его запуском, порт — нет: перед
	// подключением переспрашиваем порт, как это делает pagectl. Свою сессию
	// rodkit start держит сам, там переспрашивать нечего.
	if !s.Managed {
		if u, err := cdp.Resolve(s.Control); err == nil && u != s.Control {
			s.Control, s.Tab = u, ""
			s.save()
		}
	}
	base := s.Base
	if absolute(target) {
		base = ""
	}
	return browser.Attach(t, s.Control, base), s
}

// absolute — есть ли у адреса своя схема. Однобуквенная схема — это диск
// Windows (C:/…), а не адрес.
func absolute(target string) bool {
	u, err := url.Parse(target)
	return err == nil && len(u.Scheme) > 1
}

// currentID — вкладка, с которой работаем: выбранная через use, если она ещё
// открыта, иначе последняя.
func currentID(targets []browser.Target, s session) string {
	if len(targets) == 0 {
		fatal("нет открытых вкладок — сначала rodkit open <адрес>")
	}
	for _, tg := range targets {
		if tg.ID == s.Tab {
			return tg.ID
		}
	}
	return targets[len(targets)-1].ID
}

// current подключается к текущей вкладке — и только к ней: подключение ко
// всем сразу зависло бы на первой же выгруженной.
func current(t wait.TB, b *browser.Browser, s session) *browser.Tab {
	return b.Tab(t, currentID(b.Targets(t), s))
}

func goTo(args []string, fresh bool) {
	if len(args) != 1 {
		fatal("нужен адрес: rodkit open <url|путь>")
	}
	t := &shellTB{}
	b, s := attach(t, args[0])

	var tab *browser.Tab
	if fresh || len(b.Targets(t)) == 0 {
		tab = b.Open(t, args[0])
	} else {
		tab = current(t, b, s)
		tab.Goto(args[0])
	}
	s.Tab = tab.TargetID()
	s.save()
	fmt.Println(tab.URL())
}

func listTabs() {
	t := &shellTB{}
	b, s := attach(t, "")
	targets := b.Targets(t)
	if len(targets) == 0 {
		fmt.Println("вкладок нет")
		return
	}
	cur := currentID(targets, s)
	for i, tg := range targets {
		mark := " "
		if tg.ID == cur {
			mark = "*"
		}
		fmt.Printf("%s %d %s\n", mark, i, tg.URL)
	}
}

func use(args []string) {
	if len(args) != 1 {
		fatal("нужен номер вкладки: rodkit use <n>")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		fatal("номер вкладки — число, а не %q", args[0])
	}
	t := &shellTB{}
	b, s := attach(t, "")
	targets := b.Targets(t)
	if n < 0 || n >= len(targets) {
		fatal("вкладки %d нет, открыто %d", n, len(targets))
	}
	// Выбрать вкладку значит и показать её: в фоновую не проходит ввод, а
	// восстановленная после перезапуска без активации даже не загружена.
	ctx, cancel := context.WithTimeout(context.Background(), timeout())
	defer cancel()
	b.Activate(ctx, t, targets[n].ID)
	s.Tab = targets[n].ID
	s.save()
	fmt.Println(targets[n].URL)
}

// ---------- действия над текущей вкладкой ----------

func onTab(cmd string, args []string) {
	t := &shellTB{}
	b, s := attach(t, "")
	tab := current(t, b, s)

	ctx, cancel := context.WithTimeout(context.Background(), timeout())
	defer cancel()

	switch cmd {
	case "click":
		find(ctx, tab, arg(args, 0, "цель"), true).Click(ctx)
	case "fill":
		if len(args) != 2 {
			fatal("нужны цель и значение: rodkit fill <цель> <значение>")
		}
		find(ctx, tab, args[0], false).Fill(ctx, args[1])
	case "expect":
		el := find(ctx, tab, arg(args, 0, "цель"), false)
		fmt.Println(strings.TrimSpace(el.Text(ctx)))
	case "gone":
		role, name, css := parseTarget(arg(args, 0, "цель"))
		if css != "" {
			fatal("gone умеет только role/… и text/…")
		}
		tab.ExpectGone(ctx, args[0], role, name)
	case "url":
		fmt.Println(tab.URL())
	case "text":
		if len(args) == 1 {
			el, ok := tab.Element(ctx, args[0])
			if !ok {
				fatal("не найден %s", args[0])
			}
			fmt.Println(el.Text(ctx))
			return
		}
		fmt.Println(tab.Text(ctx))
	case "state":
		fmt.Println(tab.State())
	case "shot":
		path := filepath.Join(home(), "shot.png")
		if len(args) == 1 {
			path = args[0]
		}
		tab.Screenshot(path)
		fmt.Println(path)
	case "dump":
		tab.Dump("по команде")
	case "eval":
		fmt.Println(tab.Eval(ctx, arg(args, 0, "выражение")))
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
}

func arg(args []string, i int, what string) string {
	if len(args) <= i {
		fatal("не хватает аргумента: %s", what)
	}
	return args[i]
}

// find ждёт цель, пока та не станет видимой (и активной, если по ней кликают).
func find(ctx context.Context, tab *browser.Tab, target string, clickable bool) *browser.Element {
	role, name, css := parseTarget(target)
	switch {
	case css != "":
		return tab.ExpectVisible(ctx, css, css)
	case role == "text":
		return tab.ExpectTextVisible(ctx, target, name)
	case clickable:
		return tab.ExpectRoleEnabled(ctx, target, role, name)
	default:
		return tab.ExpectRoleVisible(ctx, target, role, name)
	}
}

// parseTarget разбирает цель: role/<роль>/<регулярка>, text/<регулярка> или CSS.
func parseTarget(target string) (role string, name *regexp.Regexp, css string) {
	parts := strings.SplitN(target, "/", 3)
	switch {
	case parts[0] == "role" && len(parts) >= 2:
		role = parts[1]
	case parts[0] == "text" && len(parts) >= 2:
		role, parts = "text", []string{"text", "", strings.TrimPrefix(target, "text/")}
	default:
		return "", nil, target
	}
	if len(parts) == 3 && parts[2] != "" {
		re, err := regexp.Compile(parts[2])
		if err != nil {
			fatal("регулярка %q: %v", parts[2], err)
		}
		name = re
	}
	if role == "text" && name == nil {
		fatal("text/ без регулярки ничего не ищет")
	}
	return role, name, ""
}

func timeout() time.Duration {
	if raw := os.Getenv("RODKIT_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			fatal("RODKIT_TIMEOUT=%q: %v", raw, err)
		}
		return d
	}
	return 10 * time.Second
}

// ---------- прогон вне теста ----------

// shellTB подставляется вместо *testing.T: падение ожидания завершает команду,
// а дампы складываются в каталог сессии.
type shellTB struct{}

func (*shellTB) Helper()                           {}
func (*shellTB) Cleanup(func())                    {}
func (*shellTB) Failed() bool                      { return false }
func (*shellTB) ArtifactDir() string               { return filepath.Join(home(), "dumps") }
func (*shellTB) Logf(format string, args ...any)   { fmt.Fprintf(os.Stderr, format+"\n", args...) }
func (*shellTB) Fatalf(format string, args ...any) { fatal(format, args...) }

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
