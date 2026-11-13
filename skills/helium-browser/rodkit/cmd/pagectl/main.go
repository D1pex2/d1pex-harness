// Команда pagectl — браузер для агента без выполнения JS на странице.
//
// От rodkit отличается ровно одним, но принципиальным: здесь нет и не может
// быть команды eval. Всё делается доменами DOM, Page, Input и Target, то есть
// внутри Blink, мимо JS-окружения страницы. Сайт не видит ни впрыснутого кода,
// ни обращений к своему document.querySelector, ни событий с isTrusted: false.
//
// Подключается к уже работающему браузеру: адрес CDP берётся из того же
// ~/.rodkit/session.json, что и у rodkit, поэтому оба инструмента работают с
// одной сессией и живым профилем пользователя.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-rod/rod/lib/proto"

	"rodkit/cdp"
)

const usage = `pagectl — страница без JS

  pagectl connect [порт|адрес]  запомнить браузер на порту отладки (9222)

  pagectl tabs                  список вкладок, * — текущая
  pagectl use <n>               сделать вкладку текущей и показать её
  pagectl open <url>            перейти в текущей вкладке
  pagectl newtab <url>          открыть новую вкладку
  pagectl url                   адрес текущей вкладки

  pagectl snapshot              что есть на странице, со ссылками [eN]
  pagectl find <запрос>         поиск: CSS, XPath или просто текст
  pagectl text                  видимый текст страницы
  pagectl shot [файл.png]       снимок экрана

  pagectl click <eN|запрос>     подвести курсор и нажать
  pagectl type <eN|запрос> <текст>   кликнуть в поле и напечатать

Ссылки [eN] живут до перехода на другую страницу. Протухли — снимите снимок
заново. Команды eval здесь нет намеренно.

Браузер берётся из ~/.rodkit/session.json, без него — с 127.0.0.1:9222.
PAGECTL_CONTROL_URL (порт или адрес) перекрывает и то, и другое.
Срок на команду — PAGECTL_TIMEOUT (30s; type добавляет по 300ms на символ).
`

// hiddenHint объясняет самую частую причину истёкшего срока. Движение мыши
// Chrome подтверждает только на следующем кадре, а невидимая вкладка кадров не
// рисует — и ввод висит, пока вкладку не покажут.
const hiddenHint = `Обычно браузер молчит из-за невидимой вкладки: она не рисует кадров, а ввод
Chrome подтверждает только на кадре. Окно не должно быть свёрнуто, вкладка —
активна (pagectl use <n>). В Windows невидимым считается и окно, перекрытое
другими, если Helium поднят без --disable-backgrounding-occluded-windows.`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("не уложились в срок PAGECTL_TIMEOUT: %w", err)
			// Домен DOM отвечает и на невидимой вкладке; висит на ней только ввод.
			if cmd := os.Args[1]; cmd == "click" || cmd == "type" {
				err = fmt.Errorf("%w\n%s", err, hiddenHint)
			}
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return nil
	}

	limit, err := deadline(cmd, args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	if cmd == "connect" {
		return connect(ctx, args)
	}

	sess, err := attach(ctx)
	if err != nil {
		return err
	}
	state := loadState()

	// «use» и «tabs» к текущей вкладке не подключаются: она могла закрыться, а
	// именно этими командами выбирают новую.
	switch cmd {
	case "use":
		return useTab(sess, state, args)
	case "tabs":
		return listTabs(sess, state)
	}

	tab, err := currentTab(sess, state, cmd, args)
	if err != nil {
		return err
	}

	switch cmd {
	case "open":
		if len(args) != 1 {
			return fmt.Errorf("нужен адрес: pagectl open <url>")
		}
		return tab.Navigate(args[0])
	case "newtab":
		state.Tab = tab.ID
		saveState(state)
		fmt.Println(tab.ID)
		return nil
	case "url":
		u, err := tab.CurrentURL()
		if err != nil {
			return err
		}
		fmt.Println(u)
		return nil
	case "text":
		s, err := tab.Text()
		if err != nil {
			return err
		}
		fmt.Println(s)
		return nil
	case "snapshot":
		return snapshot(tab, state)
	case "find":
		if len(args) != 1 {
			return fmt.Errorf("нужен запрос: pagectl find <css|xpath|текст>")
		}
		return find(tab, args[0])
	case "shot":
		return shot(tab, args)
	case "click":
		if len(args) != 1 {
			return fmt.Errorf("нужна цель: pagectl click <eN|запрос>")
		}
		id, err := resolve(tab, state, args[0])
		if err != nil {
			return err
		}
		return tab.Click(id)
	case "type":
		if len(args) != 2 {
			return fmt.Errorf("нужны цель и текст: pagectl type <eN|запрос> <текст>")
		}
		id, err := resolve(tab, state, args[0])
		if err != nil {
			return err
		}
		return tab.Type(id, args[1])
	}
	return fmt.Errorf("неизвестная команда %q\n\n%s", cmd, usage)
}

// deadline — срок на всю команду. Нужен не ради скорости: вкладка, которая не
// рисует кадров, не подтверждает ввод, и без срока команда висела бы вечно.
// Набор идёт посимвольно с паузами, поэтому type получает запас на символ.
func deadline(cmd string, args []string) (time.Duration, error) {
	d := 30 * time.Second
	if raw := os.Getenv("PAGECTL_TIMEOUT"); raw != "" {
		var err error
		if d, err = time.ParseDuration(raw); err != nil {
			return 0, fmt.Errorf("PAGECTL_TIMEOUT=%q: %w", raw, err)
		}
	}
	if cmd == "type" && len(args) == 2 {
		d += time.Duration(utf8.RuneCountInString(args[1])) * 300 * time.Millisecond
	}
	return d, nil
}

// currentTab отдаёт вкладку, с которой работаем. newtab — единственная команда,
// которая её создаёт.
func currentTab(sess *cdp.Session, state *State, cmd string, args []string) (*cdp.Tab, error) {
	if cmd == "newtab" {
		if len(args) != 1 {
			return nil, fmt.Errorf("нужен адрес: pagectl newtab <url>")
		}
		return sess.NewTab(args[0])
	}
	return sess.Tab(state.Tab)
}

func listTabs(sess *cdp.Session, state *State) error {
	tabs, err := sess.Tabs()
	if err != nil {
		return err
	}
	for i, t := range tabs {
		mark := " "
		if t.ID == state.Tab || (state.Tab == "" && i == 0) {
			mark = "*"
		}
		fmt.Printf("%s %d %s\n", mark, i, t.URL)
	}
	return nil
}

func useTab(sess *cdp.Session, state *State, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("нужен номер: pagectl use <n>")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("номер вкладки должен быть числом: %q", args[0])
	}
	tabs, err := sess.Tabs()
	if err != nil {
		return err
	}
	if n < 0 || n >= len(tabs) {
		return fmt.Errorf("вкладки %d нет, всего %d", n, len(tabs))
	}
	// Выбрать вкладку значит и показать её: в фоновой ввод не пройдёт.
	if err := sess.Activate(tabs[n].ID); err != nil {
		return err
	}
	state.Tab = tabs[n].ID
	state.Refs = nil // снимок был для другой страницы
	saveState(state)
	fmt.Println(tabs[n].URL)
	return nil
}

func snapshot(tab *cdp.Tab, state *State) error {
	nodes, err := tab.Snapshot()
	if err != nil {
		return err
	}
	state.Tab = tab.ID
	state.Refs = map[string]proto.DOMBackendNodeID{}
	for _, n := range nodes {
		state.Refs[n.Ref] = n.Backend
		name := n.Name
		if name != "" {
			name = `"` + name + `"`
		}
		fmt.Printf("[%-4s] %-10s %-40s %s\n", n.Ref, n.Kind, name, n.Tag)
	}
	saveState(state)
	if len(nodes) == 0 {
		fmt.Println("(на странице нет ничего, с чем можно взаимодействовать)")
	}
	return nil
}

func find(tab *cdp.Tab, query string) error {
	ids, err := tab.Search(query)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("ничего не найдено по %q", query)
	}
	for _, id := range ids {
		box, err := tab.Box(id)
		if err != nil {
			fmt.Printf("%d  (не отрисован)\n", id)
			continue
		}
		fmt.Printf("%d  %.0fx%.0f в (%.0f, %.0f)\n", id, box.Width, box.Height, box.X, box.Y)
	}
	return nil
}

func shot(tab *cdp.Tab, args []string) error {
	path := "shot.png"
	if len(args) == 1 {
		path = args[0]
	}
	data, err := tab.Screenshot()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("запись снимка: %w", err)
	}
	fmt.Println(path)
	return nil
}

// resolve превращает аргумент в узел: либо ссылка из последнего снимка, либо
// запрос, который ищется прямо сейчас.
func resolve(tab *cdp.Tab, state *State, target string) (proto.DOMBackendNodeID, error) {
	if looksLikeRef(target) {
		id, ok := state.Refs[target]
		if !ok {
			return 0, fmt.Errorf("ссылки %s нет в последнем снимке, снимите заново: pagectl snapshot", target)
		}
		if _, err := tab.Box(id); err != nil {
			return 0, fmt.Errorf("%s протух (страница сменилась), снимите заново: pagectl snapshot", target)
		}
		return id, nil
	}
	ids, err := tab.Search(target)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := tab.Box(id); err == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("по %q не нашлось ни одного отрисованного элемента", target)
}

func looksLikeRef(s string) bool {
	if len(s) < 2 || s[0] != 'e' {
		return false
	}
	_, err := strconv.Atoi(s[1:])
	return err == nil
}

// State — что помнится между запусками: текущая вкладка и ссылки последнего
// снимка. Отдельный файл, чтобы не мешать rodkit в его session.json.
type State struct {
	Tab  string                            `json:"tab"`
	Refs map[string]proto.DOMBackendNodeID `json:"refs"`
}

func stateDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rodkit")
}

func loadState() *State {
	s := &State{}
	data, err := os.ReadFile(filepath.Join(stateDir(), "pagectl.json"))
	if err == nil {
		_ = json.Unmarshal(data, s)
	}
	return s
}

func saveState(s *State) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.MkdirAll(stateDir(), 0o755)
	_ = os.WriteFile(filepath.Join(stateDir(), "pagectl.json"), data, 0o644)
}

// ---------- подключение ----------

// sessionFile — файл сессии rodkit. Адрес CDP берём из него, чтобы оба
// инструмента смотрели в один браузер. Поля rodkit (base_url, tab) pagectl
// переносит как есть.
type sessionFile struct {
	PID     int    `json:"pid"`
	Control string `json:"control_url"`
	Base    string `json:"base_url"`
	Tab     string `json:"tab"`
	Managed bool   `json:"managed,omitempty"`
}

func sessionPath() string { return filepath.Join(stateDir(), "session.json") }

func loadSession() sessionFile {
	var s sessionFile
	if data, err := os.ReadFile(sessionPath()); err == nil {
		// BOM оставляет PowerShell 5, если файл писали его Out-File -Encoding utf8.
		_ = json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &s)
	}
	s.Control = strings.TrimSpace(s.Control)
	return s
}

func saveSession(s sessionFile) error {
	data, _ := json.MarshalIndent(s, "", " ")
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return fmt.Errorf("каталог сессии: %w", err)
	}
	if err := os.WriteFile(sessionPath(), data, 0o644); err != nil {
		return fmt.Errorf("запись сессии: %w", err)
	}
	return nil
}

// attach подключается к браузеру из файла сессии, а без файла — к порту
// отладки по умолчанию.
//
// Адрес CDP несёт id браузера и меняется с каждым его запуском: после
// перезапуска Helium файл протухает, хотя порт тот же. Тогда адрес заново
// спрашивается у порта и файл переписывается — отдельный шаг
// «переподключиться» агенту не нужен.
//
// Пустой адрес rod понимает как «запусти свой браузер», поэтому до cdp.Attach
// он не доходит никогда.
func attach(ctx context.Context) (*cdp.Session, error) {
	if env := os.Getenv("PAGECTL_CONTROL_URL"); env != "" {
		u := env
		if !strings.HasPrefix(u, "ws://") && !strings.HasPrefix(u, "wss://") {
			var err error
			if u, err = cdp.Resolve(u); err != nil {
				return nil, err
			}
		}
		return cdp.Attach(ctx, u)
	}

	known := loadSession()
	var attachErr error
	if known.Control != "" {
		sess, err := cdp.Attach(ctx, known.Control)
		if err == nil {
			return sess, nil
		}
		attachErr = err
	}
	// Сессия rodkit start живёт со своим браузером на случайном порту: раз её
	// нет, переподключаться там не к кому — идём на порт по умолчанию.
	if known.Managed {
		known = sessionFile{}
	}
	u, err := cdp.Resolve(known.Control)
	if err != nil {
		return nil, fmt.Errorf("%w\nбраузер не открыт с портом отладки — поднимите его с --remote-debugging-port или укажите порт: pagectl connect <порт>", err)
	}
	// Тот же адрес — значит, браузер тот же, и сбой был не в адресе: текущую
	// вкладку и ссылки снимка сбрасывать не за что.
	if u == known.Control {
		return nil, attachErr
	}
	return remember(ctx, u, known)
}

// connect запоминает браузер, который уже слушает порт отладки.
func connect(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("нужен один порт или адрес: pagectl connect [порт|адрес]")
	}
	addr := ""
	if len(args) == 1 {
		addr = args[0]
	}
	u, err := cdp.Resolve(addr)
	if err != nil {
		return err
	}
	// Файл пишется с нуля: base_url прежней сессии rodkit к этому браузеру
	// отношения не имеет.
	sess, err := remember(ctx, u, sessionFile{})
	if err != nil {
		return err
	}
	fmt.Printf("%s, pid %d\n%s\n", sess.Product(), sess.BrowserPID(), u)
	return nil
}

// remember подключается по свежему адресу и записывает его в файл сессии
// поверх s. Текущая вкладка и ссылки снимка принадлежали прежнему запуску
// браузера, поэтому сбрасываются.
func remember(ctx context.Context, u string, s sessionFile) (*cdp.Session, error) {
	sess, err := cdp.Attach(ctx, u)
	if err != nil {
		return nil, err
	}
	// Managed снимаем: браузер на порту чужой, и rodkit stop его не тронет.
	s.PID, s.Control, s.Tab, s.Managed = sess.BrowserPID(), u, "", false
	if err := saveSession(s); err != nil {
		return nil, err
	}
	saveState(&State{})
	return sess, nil
}
