package cdp

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

// Снимок страницы для агента. Дерево доступности не используем сознательно:
// Accessibility.getFullAXTree переводит рендерер в режим скринридера, а это
// настоящее изменение состояния браузера ради удобства вывода. Вместо него
// DOM.getDocument и вычисление подписей на нашей стороне — чуть грубее в
// экзотической разметке, зато ни одного лишнего включённого домена.

// Node — строка снимка: то, с чем агент может что-то сделать.
type Node struct {
	Ref     string                 `json:"ref"`
	Kind    string                 `json:"kind"`
	Name    string                 `json:"name"`
	Tag     string                 `json:"tag"`
	Backend proto.DOMBackendNodeID `json:"backend"`
}

const maxName = 70

// Snapshot обходит дерево, отбирает узлы, с которыми можно взаимодействовать, и
// оставляет только реально отрисованные — видимость проверяется через
// getBoxModel, единственный доступный без JS способ.
func (t *Tab) Snapshot() ([]Node, error) {
	doc, err := t.Document()
	if err != nil {
		return nil, err
	}

	labels := map[string]string{} // id поля -> текст его <label for>
	collectLabels(doc, labels)

	var out []Node
	n := 0
	walk(doc, func(node *proto.DOMNode, skip *bool) {
		tag := strings.ToLower(node.LocalName)
		if isHidden(tag, node) {
			*skip = true
			return
		}
		kind := kindOf(tag, node)
		if kind == "" {
			return
		}
		if _, err := t.Box(node.BackendNodeID); err != nil {
			return // не отрисован — агенту он не нужен
		}
		n++
		out = append(out, Node{
			Ref:     fmt.Sprintf("e%d", n),
			Kind:    kind,
			Name:    nameOf(node, tag, labels),
			Tag:     descriptor(node, tag),
			Backend: node.BackendNodeID,
		})
	})
	return out, nil
}

// walk обходит дерево целиком, включая shadow DOM и содержимое фреймов.
// skip=true у обработчика означает «внутрь не ходить».
func walk(n *proto.DOMNode, fn func(*proto.DOMNode, *bool)) {
	if n == nil {
		return
	}
	if n.NodeType == 1 { // element
		skip := false
		fn(n, &skip)
		if skip {
			return
		}
	}
	for _, c := range n.Children {
		walk(c, fn)
	}
	for _, r := range n.ShadowRoots {
		walk(r, fn)
	}
	walk(n.ContentDocument, fn)
}

var hiddenTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "head": true,
	"meta": true, "link": true, "template": true, "svg": true,
}

func isHidden(tag string, n *proto.DOMNode) bool {
	if hiddenTags[tag] {
		return true
	}
	if attr(n, "hidden") != "" || attr(n, "aria-hidden") == "true" {
		return true
	}
	// Инлайновый display:none ловим здесь; спрятанное через класс отсеет
	// последующая проверка getBoxModel.
	return strings.Contains(strings.ReplaceAll(attr(n, "style"), " ", ""), "display:none")
}

// kindOf — что это за элемент с точки зрения агента. Пустая строка означает
// «в снимок не попадает».
func kindOf(tag string, n *proto.DOMNode) string {
	switch tag {
	case "a":
		if attr(n, "href") == "" {
			return ""
		}
		return "link"
	case "button":
		return "button"
	case "select":
		return "select"
	case "textarea":
		return "textbox"
	case "summary":
		return "disclosure"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "input":
		switch strings.ToLower(attr(n, "type")) {
		case "hidden":
			return ""
		case "checkbox":
			return "checkbox"
		case "radio":
			return "radio"
		case "submit", "button", "reset", "image":
			return "button"
		default:
			return "textbox"
		}
	}
	if attr(n, "contenteditable") == "true" || attr(n, "contenteditable") == "" && hasAttr(n, "contenteditable") {
		return "textbox"
	}
	if role := attr(n, "role"); role != "" {
		return role
	}
	return ""
}

func hasAttr(n *proto.DOMNode, name string) bool {
	for i := 0; i+1 < len(n.Attributes); i += 2 {
		if strings.EqualFold(n.Attributes[i], name) {
			return true
		}
	}
	return false
}

// nameOf приближает вычисленное имя доступности. Полная спецификация резолвит
// длинную цепочку с aria-labelledby; здесь покрыты источники, которые реально
// встречаются в формах.
func nameOf(n *proto.DOMNode, tag string, labels map[string]string) string {
	for _, s := range []string{
		attr(n, "aria-label"),
		labels[attr(n, "id")],
		attr(n, "placeholder"),
		attr(n, "alt"),
		attr(n, "title"),
	} {
		if s = clean(s); s != "" {
			return s
		}
	}
	if tag == "input" {
		if v := clean(attr(n, "value")); v != "" {
			return v
		}
		if nm := clean(attr(n, "name")); nm != "" {
			return nm
		}
	}
	return clean(textOf(n))
}

// descriptor — короткая подсказка «что это в разметке», чтобы агент мог
// сослаться на элемент селектором, если снимок протух.
func descriptor(n *proto.DOMNode, tag string) string {
	s := tag
	if id := attr(n, "id"); id != "" {
		s += "#" + id
	} else if nm := attr(n, "name"); nm != "" {
		s += "[name=" + nm + "]"
	}
	if tag == "input" {
		if tp := attr(n, "type"); tp != "" {
			s += "[type=" + tp + "]"
		}
	}
	return s
}

func collectLabels(root *proto.DOMNode, out map[string]string) {
	walk(root, func(n *proto.DOMNode, _ *bool) {
		if strings.ToLower(n.LocalName) != "label" {
			return
		}
		if for_ := attr(n, "for"); for_ != "" {
			out[for_] = clean(textOf(n))
		}
	})
}

// textOf — видимый текст поддерева.
func textOf(n *proto.DOMNode) string {
	var b strings.Builder
	var rec func(*proto.DOMNode)
	rec = func(x *proto.DOMNode) {
		if x == nil {
			return
		}
		if x.NodeType == 3 { // text
			b.WriteString(x.NodeValue)
			b.WriteString(" ")
			return
		}
		if x.NodeType == 1 && hiddenTags[strings.ToLower(x.LocalName)] {
			return
		}
		for _, c := range x.Children {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

// Text — весь видимый текст страницы. Аналог innerText, собранный из дерева.
func (t *Tab) Text() (string, error) {
	doc, err := t.Document()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	walk(doc, func(n *proto.DOMNode, skip *bool) {
		tag := strings.ToLower(n.LocalName)
		if isHidden(tag, n) {
			*skip = true
			return
		}
		if blockTags[tag] {
			b.WriteString("\n")
		}
		for _, c := range n.Children {
			if c.NodeType == 3 {
				if s := norm(c.NodeValue); s != "" {
					b.WriteString(s)
					b.WriteString(" ")
				}
			}
		}
	})
	return squeezeLines(b.String()), nil
}

var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "tr": true, "section": true, "article": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "footer": true, "nav": true, "main": true, "br": true, "form": true,
}

// norm схлопывает пробелы. Без обрезки: текст страницы режет только тот, кто
// его читает, а подписи в снимке — отдельная задача.
func norm(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// clean — norm плюс обрезка до длины подписи в снимке. Режем по рунам, иначе на
// кириллице получится битый UTF-8.
func clean(s string) string {
	s = norm(s)
	if r := []rune(s); len(r) > maxName {
		return string(r[:maxName]) + "…"
	}
	return s
}

func squeezeLines(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
