// Package cdp — работа со страницей без единой строчки JS на ней.
//
// Зачем. Всё, что агент выполняет на странице через Runtime.evaluate, оставляет
// следы: rod ищет элементы своими хелперами, те зовут document.querySelector, а
// сайт может его подменить и снять стек вызова — поле nativeFunctionsStackTraces
// в device_info.min.js делает ровно это, и в стеке видно «functions.element».
// Клик через element.click() и вовсе приходит с isTrusted: false, что детектор
// ловит первой же проверкой.
//
// Здесь вместо этого домены DOM, Page, Input и Target — они живут в C++ Blink,
// мимо JS-биндингов страницы. Домен Runtime не включается никогда: подписка на
// его события заставила бы V8 дёргать Error.prepareStackTrace на каждый
// console.log страницы, и это ровно тот сигнал, по которому детекторы ставят
// isAutomatedWithCDP.
//
// Запрет держится не обещанием, а тестом TestNoPageJS в этом пакете.
package cdp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

// Session — подключение к уже работающему браузеру. Свой браузер пакет не
// поднимает: инструмент рассчитан на живой профиль пользователя.
type Session struct {
	browser *rod.Browser
}

// Attach подключается к браузеру. Все вызовы сессии живут в ctx: его срок —
// единственное, что не даёт команде повиснуть на вкладке, которая молчит.
//
// NoDefaultDevice обязателен. Без него rod при подключении к каждой вкладке
// включает эмуляцию «ноутбука»: User-Agent становится Mac с Chrome 114,
// Accept-Language — en, вьюпорт — 1280×800, и всё это при настоящем
// navigator.platform. На живой вкладке это смена личности на время команды:
// запрос, который страница шлёт в этот момент — тот самый submit по нашему
// клику, — уходит с чужим UA. Снимается при отключении, но след уже оставлен.
func Attach(ctx context.Context, controlURL string) (*Session, error) {
	b := rod.New().NoDefaultDevice().Context(ctx).ControlURL(controlURL)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("подключение к браузеру по %s: %w", controlURL, err)
	}
	return &Session{browser: b}, nil
}

// Tab — вкладка браузера.
type Tab struct {
	page *rod.Page
	ID   string
	URL  string
}

// Tabs перечисляет вкладки через Target.getTargets.
func (s *Session) Tabs() ([]*Tab, error) {
	res, err := proto.TargetGetTargets{}.Call(s.browser)
	if err != nil {
		return nil, fmt.Errorf("список вкладок: %w", err)
	}
	var tabs []*Tab
	for _, info := range res.TargetInfos {
		if info.Type != "page" {
			continue
		}
		tabs = append(tabs, &Tab{ID: string(info.TargetID), URL: info.URL})
	}
	return tabs, nil
}

// Tab подключается к вкладке по её TargetID. Пустой id — первая вкладка.
func (s *Session) Tab(id string) (*Tab, error) {
	tabs, err := s.Tabs()
	if err != nil {
		return nil, err
	}
	if len(tabs) == 0 {
		return nil, errors.New("в браузере нет вкладок")
	}
	tab := tabs[0]
	if id != "" {
		tab = nil
		for _, t := range tabs {
			if t.ID == id {
				tab = t
				break
			}
		}
		if tab == nil {
			return nil, fmt.Errorf("текущей вкладки %s больше нет — выберите другую: pagectl tabs, затем pagectl use <n>", id)
		}
	}
	ok, err := Loaded(s.browser, proto.TargetTargetID(tab.ID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("вкладка %s не отвечает: выгружена после восстановления сессии или занята — pagectl use <n> активирует и загрузит её", tab.ID)
	}
	page, err := s.browser.PageFromTarget(proto.TargetTargetID(tab.ID))
	if err != nil {
		return nil, fmt.Errorf("подключение к вкладке %s: %w", tab.ID, err)
	}
	tab.page = page
	return tab, nil
}

// loadProbe — сколько ждать ответа рендерера. Загруженная вкладка отвечает за
// миллисекунды, даже фоновая; молчит только та, у которой рендерера нет.
const loadProbe = 2 * time.Second

// Loaded — отвечает ли рендерер вкладки.
//
// Вкладка, восстановленная после перезапуска браузера, до первой активации
// живёт без рендерера: подключиться к ней можно, но любой запрос к странице —
// начиная с Page.enable, который rod шлёт при подключении, — висит до загрузки.
// После перезапуска с --restore-last-session живой браузер почти целиком из
// таких, поэтому вопрос задаётся заранее, одним запросом с коротким дедлайном,
// через собственную сессию и мимо rod.
func Loaded(b *rod.Browser, id proto.TargetTargetID) (bool, error) {
	res, err := proto.TargetAttachToTarget{TargetID: id, Flatten: true}.Call(b)
	if err != nil {
		return false, fmt.Errorf("подключение к вкладке %s: %w", id, err)
	}
	defer func() { _ = proto.TargetDetachFromTarget{SessionID: res.SessionID}.Call(b) }()

	ctx, cancel := context.WithTimeout(b.GetContext(), loadProbe)
	defer cancel()
	_, err = proto.DOMGetDocument{Depth: gson.Int(1)}.Call(probe{b, ctx, res.SessionID})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, context.DeadlineExceeded):
		return false, nil
	default:
		return false, fmt.Errorf("вкладка %s: %w", id, err)
	}
}

// probe — сессия вкладки без обвязки rod: то, что нужно proto.Call, и ничего
// сверх. Свой контекст, чтобы дедлайн пробника не задел остальные вызовы.
type probe struct {
	browser *rod.Browser
	ctx     context.Context
	session proto.TargetSessionID
}

func (p probe) Call(ctx context.Context, sessionID, method string, params any) ([]byte, error) {
	return p.browser.Call(ctx, sessionID, method, params)
}
func (p probe) GetContext() context.Context         { return p.ctx }
func (p probe) GetSessionID() proto.TargetSessionID { return p.session }

// Activate делает вкладку активной, как щелчок по ней в полосе вкладок, и
// дожидается её рендерера. Фоновая вкладка не рисует кадров, а движение мыши
// Chrome подтверждает только на кадре — ввод в неё повис бы; восстановленная
// после перезапуска и вовсе загружается только с активации.
func (s *Session) Activate(id string) error {
	err := proto.TargetActivateTarget{TargetID: proto.TargetTargetID(id)}.Call(s.browser)
	if err != nil {
		return fmt.Errorf("активация вкладки %s: %w", id, err)
	}
	ctx := s.browser.GetContext()
	for {
		ok, err := Loaded(s.browser, proto.TargetTargetID(id))
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("вкладка %s не загрузилась после активации: %w", id, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// NewTab открывает вкладку и возвращает её.
func (s *Session) NewTab(url string) (*Tab, error) {
	res, err := proto.TargetCreateTarget{URL: url}.Call(s.browser)
	if err != nil {
		return nil, fmt.Errorf("новая вкладка: %w", err)
	}
	return s.Tab(string(res.TargetID))
}

func (t *Tab) Navigate(url string) error {
	_, err := proto.PageNavigate{URL: url}.Call(t.page)
	if err != nil {
		return fmt.Errorf("переход на %s: %w", url, err)
	}
	return nil
}

// Document — полное дерево страницы одним вызовом. Depth -1 и Pierce означают
// «со всеми потомками и внутрь shadow DOM и фреймов».
func (t *Tab) Document() (*proto.DOMNode, error) {
	res, err := proto.DOMGetDocument{Depth: gson.Int(-1), Pierce: true}.Call(t.page)
	if err != nil {
		return nil, fmt.Errorf("дерево страницы: %w", err)
	}
	return res.Root, nil
}

// Search ищет по странице силами Blink: CSS-селектор, XPath или простой текст —
// DOM.performSearch разбирает все три сам. Возвращает backendNodeId найденного.
func (t *Tab) Search(query string) ([]proto.DOMBackendNodeID, error) {
	// Дерево нужно запросить до поиска: без него у сессии нет nodeId, и
	// getSearchResults вернёт нули.
	if _, err := t.Document(); err != nil {
		return nil, err
	}
	found, err := proto.DOMPerformSearch{Query: query}.Call(t.page)
	if err != nil {
		return nil, fmt.Errorf("поиск %q: %w", query, err)
	}
	defer func() {
		_ = proto.DOMDiscardSearchResults{SearchID: found.SearchID}.Call(t.page)
	}()
	if found.ResultCount == 0 {
		return nil, nil
	}
	res, err := proto.DOMGetSearchResults{
		SearchID: found.SearchID, FromIndex: 0, ToIndex: found.ResultCount,
	}.Call(t.page)
	if err != nil {
		return nil, fmt.Errorf("результаты поиска %q: %w", query, err)
	}
	var ids []proto.DOMBackendNodeID
	for _, nodeID := range res.NodeIDs {
		desc, err := proto.DOMDescribeNode{NodeID: nodeID}.Call(t.page)
		if err != nil {
			continue // узел мог уехать между поиском и описанием
		}
		ids = append(ids, desc.Node.BackendNodeID)
	}
	return ids, nil
}

// ErrNotRendered — узел есть в дереве, но не отрисован: скрыт стилями, схлопнут
// в нулевой размер или не в потоке. Кликать по нему бессмысленно.
var ErrNotRendered = errors.New("элемент не отрисован")

// Box — прямоугольник узла во вьюпорте. Заодно единственная проверка видимости,
// доступная без JS: у скрытого элемента getBoxModel либо падает, либо отдаёт
// нулевую площадь.
func (t *Tab) Box(id proto.DOMBackendNodeID) (*proto.DOMRect, error) {
	res, err := proto.DOMGetBoxModel{BackendNodeID: id}.Call(t.page)
	if err != nil {
		return nil, ErrNotRendered
	}
	q := res.Model.Content
	if len(q) < 8 {
		return nil, ErrNotRendered
	}
	minX, minY := q[0], q[1]
	maxX, maxY := q[0], q[1]
	for i := 2; i < 8; i += 2 {
		minX = min(minX, q[i])
		maxX = max(maxX, q[i])
		minY = min(minY, q[i+1])
		maxY = max(maxY, q[i+1])
	}
	rect := &proto.DOMRect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
	if rect.Width <= 0 || rect.Height <= 0 {
		return nil, ErrNotRendered
	}
	return rect, nil
}

// ScrollTo подтягивает узел во вьюпорт. CDP-шный аналог scrollIntoView, без JS.
func (t *Tab) ScrollTo(id proto.DOMBackendNodeID) error {
	err := proto.DOMScrollIntoViewIfNeeded{BackendNodeID: id}.Call(t.page)
	if err != nil {
		return ErrNotRendered
	}
	return nil
}

func (t *Tab) Screenshot() ([]byte, error) {
	res, err := proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng}.Call(t.page)
	if err != nil {
		return nil, fmt.Errorf("снимок экрана: %w", err)
	}
	return res.Data, nil
}

// CurrentURL — адрес вкладки, прочитанный из дерева, а не из location.
func (t *Tab) CurrentURL() (string, error) {
	doc, err := t.Document()
	if err != nil {
		return "", err
	}
	return doc.DocumentURL, nil
}

// attr достаёт атрибут из плоского списка [имя, значение, имя, значение, ...].
func attr(n *proto.DOMNode, name string) string {
	for i := 0; i+1 < len(n.Attributes); i += 2 {
		if strings.EqualFold(n.Attributes[i], name) {
			return n.Attributes[i+1]
		}
	}
	return ""
}
