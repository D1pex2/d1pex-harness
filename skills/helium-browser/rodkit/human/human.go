// Package human — ввод, который детектор поведения принимает за живого
// человека. События из CDP и так приходят на страницу с isTrusted: true, их не
// отличить от настоящей мыши; палит другое — траектория и тайминги.
//
// Что именно проверяют, видно в device_info.min.js у deviceandbrowserinfo:
// hasClicked<Поле>ExactCenter (допуск 1px), ...SmallMargin (3px),
// <поле>FieldHasClick, numSuspiciousKeyEvents<Поле>, typeAtWithModifier.
//
// Пакет ничего не знает про поиск элементов: на вход — прямоугольник и текст.
// Поэтому им пользуются оба инструмента, и тот, что ищет через JS, и тот, что
// через CDP.
package human

import (
	"math"
	"math/rand"
	"strings"
	"time"
	"unicode"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// Jitter — случайная задержка в диапазоне. Постоянные интервалы между
// действиями видны в статистике не хуже прямой линии курсора.
func Jitter(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

// ClickPoint — точка внутри элемента, заведомо дальше 3px от центра.
func ClickPoint(box *proto.DOMRect) proto.Point {
	off := func(size float64) float64 {
		// ponytail: у элемента уже 8px по оси уйти от центра дальше 3px, не
		// вылезая наружу, нельзя — такие цели кликаем как получится.
		max := size/2 - 1
		if max <= 0 {
			return 0
		}
		// 5, а не 3: клик сравнивают с Math.round(центра), а сами координаты
		// Chrome округляет до целых — на 3.5px промах схлопывается в 3 и
		// попадает в их SmallMargin.
		min := 5.0
		if min > max {
			min = max
		}
		d := min + rand.Float64()*(max-min)
		if rand.Intn(2) == 0 {
			return -d
		}
		return d
	}
	return proto.Point{
		X: box.X + box.Width/2 + off(box.Width),
		Y: box.Y + box.Height/2 + off(box.Height),
	}
}

func bezier(p0, p1, p2, p3 proto.Point, t float64) proto.Point {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return proto.Point{
		X: a*p0.X + b*p1.X + c*p2.X + d*p3.X,
		Y: a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y,
	}
}

// easeInOut — разгон и торможение. Рука не двигается с постоянной скоростью.
func easeInOut(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - math.Pow(-2*t+2, 2)/2
}

// Move ведёт курсор к точке дугой со случайным прогибом. Контрольные точки
// Безье уводятся по перпендикуляру к отрезку — получается кривая, а не прямая,
// и на каждом шаге ещё дрожание в пару пикселей.
func Move(page *rod.Page, to proto.Point) error {
	from := page.Mouse.Position()
	dx, dy := to.X-from.X, to.Y-from.Y
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return page.Mouse.MoveTo(to)
	}

	nx, ny := -dy/dist, dx/dist // единичный перпендикуляр к направлению
	bow := func() float64 {
		return (rand.Float64()*2 - 1) * dist * 0.12 // прогиб до 12% пути
	}
	b1, b2 := bow(), bow()
	c1 := proto.Point{X: from.X + dx*0.3 + nx*b1, Y: from.Y + dy*0.3 + ny*b1}
	c2 := proto.Point{X: from.X + dx*0.7 + nx*b2, Y: from.Y + dy*0.7 + ny*b2}

	steps := 15 + rand.Intn(15)
	i := 0
	return page.Mouse.MoveAlong(func() (proto.Point, bool) {
		i++
		if i >= steps {
			return to, true
		}
		p := bezier(from, c1, c2, to, easeInOut(float64(i)/float64(steps)))
		p.X += rand.Float64()*2 - 1
		p.Y += rand.Float64()*2 - 1
		time.Sleep(Jitter(4*time.Millisecond, 12*time.Millisecond))
		return p, false
	})
}

// ClickAt подводит курсор к прямоугольнику дугой, выжидает и жмёт. Пауза между
// нажатием и отпусканием — то, что у живого человека никогда не равно нулю.
func ClickAt(page *rod.Page, box *proto.DOMRect) error {
	if err := Move(page, ClickPoint(box)); err != nil {
		return err
	}
	time.Sleep(Jitter(50*time.Millisecond, 200*time.Millisecond))
	if err := page.Mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	time.Sleep(Jitter(40*time.Millisecond, 110*time.Millisecond))
	return page.Mouse.Up(proto.InputMouseButtonLeft, 1)
}

// ClearField стирает содержимое поля под фокусом: выделить всё и Backspace.
//
// Прочитать текущее значение без JS нельзя (в атрибуте лежит только начальное),
// поэтому «сколько раз жать Backspace» не вычислить — выделение решает это
// одним движением. Комбинация с модификатором безопасна: keypress на ней не
// возникает, а hasModifierKeyPressed детектор смотрит именно в keypress.
func ClearField(page *rod.Page) error {
	selectAll := input.ControlLeft
	if input.IsMac {
		selectAll = input.MetaLeft
	}
	if err := page.Keyboard.Press(selectAll); err != nil {
		return err
	}
	if err := page.Keyboard.Type(input.KeyA); err != nil {
		_ = page.Keyboard.Release(selectAll)
		return err
	}
	if err := page.Keyboard.Release(selectAll); err != nil {
		return err
	}
	time.Sleep(Jitter(40*time.Millisecond, 120*time.Millisecond))
	return page.Keyboard.Type(input.Backspace)
}

// TypeText печатает посимвольно настоящими key-событиями. Поле должно быть уже
// в фокусе. Input.insertText, которым набирает rod.Element.Input, текст
// подставляет, но keypress не порождает вовсе — для детектора это поле,
// заполненное без набора.
func TypeText(page *rod.Page, text string) error {
	for _, r := range text {
		var err error
		switch {
		case onUSLayout(r):
			err = typeRune(page, r)
		case isCyrillic(r):
			err = typeCyrillic(page, r)
		default:
			// ponytail: эмодзи и прочую экзотику вставляем текстом — раскладки
			// под них не существует, keypress человек тоже не порождает (их
			// вставляют из палитры), так что подделывать нечего.
			err = page.InsertText(string(r))
		}
		if err != nil {
			return err
		}
		time.Sleep(Jitter(60*time.Millisecond, 180*time.Millisecond))
	}
	return nil
}

// onUSLayout — есть ли клавиша под этот символ у rod. Проверять приходится до
// вызова: input.Key.Info() на незнакомой клавише не возвращает ошибку, а
// паникует («key not defined»), и никакой fallback после неё не сработает.
// Верхняя граница строгая: MaxASCII это 0x7F, то есть DEL, и клавиши под него
// в keymap нет.
func onUSLayout(r rune) bool {
	return r >= 0x20 && r < unicode.MaxASCII
}

func isCyrillic(r rune) bool {
	return (r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') || r == 'ё' || r == 'Ё'
}

// shiftedRunes — символы, которые на US-раскладке набираются только с Shift.
const shiftedRunes = "~!@#$%^&*()_+{}|:\"<>?"

// typeRune жмёт Shift там, где его жал бы человек. rod кодирует «@» как Digit2
// с правильным keyCode, но shiftKey в событии остаётся false — а «@» без Shift
// на клавиатуре набрать нельзя, и детектор это знает: typeAtCharacter: true
// при typeAtWithModifier: false — верный признак бота.
func typeRune(page *rod.Page, r rune) error {
	if !unicode.IsUpper(r) && !strings.ContainsRune(shiftedRunes, r) {
		return page.Keyboard.Type(input.Key(r))
	}
	if err := page.Keyboard.Press(input.ShiftLeft); err != nil {
		return err
	}
	defer func() { _ = page.Keyboard.Release(input.ShiftLeft) }()
	time.Sleep(Jitter(20*time.Millisecond, 60*time.Millisecond))
	return page.Keyboard.Type(input.Key(r))
}
