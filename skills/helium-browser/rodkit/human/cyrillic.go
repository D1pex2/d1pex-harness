package human

import (
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// Кириллицу нельзя набирать через Page.InsertText: текст появляется, но ни
// одного keypress не возникает, и для детектора поле оказывается заполненным
// без набора — та же дыра, что с rod.Element.Input. Клавиш под кириллицу у rod
// нет, поэтому событие собираем руками.
//
// Физическая клавиша при этом должна быть настоящей: человек с ЙЦУКЕН, набирая
// «а», жмёт ту же кнопку, на которой у него латинская F, и браузер отдаёт
// code: "KeyF" с keyCode 70. Пустой code или код не от той клавиши — уже
// несогласованность.

// ruKey — физическая клавиша US-раскладки под русской буквой.
type ruKey struct {
	code string
	vk   int // windowsVirtualKeyCode физической клавиши
}

// ruLayout — стандартная ЙЦУКЕН. Ключи только строчные; заглавные получаются
// той же клавишей с Shift, как на настоящей клавиатуре.
var ruLayout = map[rune]ruKey{
	'ё': {"Backquote", 192},
	'й': {"KeyQ", 81}, 'ц': {"KeyW", 87}, 'у': {"KeyE", 69}, 'к': {"KeyR", 82},
	'е': {"KeyT", 84}, 'н': {"KeyY", 89}, 'г': {"KeyU", 85}, 'ш': {"KeyI", 73},
	'щ': {"KeyO", 79}, 'з': {"KeyP", 80}, 'х': {"BracketLeft", 219}, 'ъ': {"BracketRight", 221},
	'ф': {"KeyA", 65}, 'ы': {"KeyS", 83}, 'в': {"KeyD", 68}, 'а': {"KeyF", 70},
	'п': {"KeyG", 71}, 'р': {"KeyH", 72}, 'о': {"KeyJ", 74}, 'л': {"KeyK", 75},
	'д': {"KeyL", 76}, 'ж': {"Semicolon", 186}, 'э': {"Quote", 222},
	'я': {"KeyZ", 90}, 'ч': {"KeyX", 88}, 'с': {"KeyC", 67}, 'м': {"KeyV", 86},
	'и': {"KeyB", 66}, 'т': {"KeyN", 78}, 'ь': {"KeyM", 77}, 'б': {"Comma", 188},
	'ю': {"Period", 190},
}

// lowerRU — строчная пара для русской заглавной. unicode.ToLower тут подходит,
// отдельная таблица не нужна, но «Ё» стоит вне основного диапазона, поэтому
// проверяем по самой раскладке.
func lookupRU(r rune) (ruKey, bool, bool) {
	if k, ok := ruLayout[r]; ok {
		return k, false, true
	}
	lower := toLowerRU(r)
	if k, ok := ruLayout[lower]; ok {
		return k, true, true
	}
	return ruKey{}, false, false
}

func toLowerRU(r rune) rune {
	switch {
	case r == 'Ё':
		return 'ё'
	case r >= 'А' && r <= 'Я':
		return r + ('а' - 'А')
	}
	return r
}

// typeCyrillic шлёт keyDown с текстом и keyUp — этого достаточно, чтобы Blink
// сгенерировал keypress с настоящими key, code и keyCode.
func typeCyrillic(page *rod.Page, r rune) error {
	k, needShift, ok := lookupRU(r)
	if !ok {
		return page.InsertText(string(r))
	}
	if needShift {
		if err := page.Keyboard.Press(input.ShiftLeft); err != nil {
			return err
		}
		defer func() { _ = page.Keyboard.Release(input.ShiftLeft) }()
		time.Sleep(Jitter(20*time.Millisecond, 60*time.Millisecond))
	}

	mods := 0
	if needShift {
		mods = input.ModifierShift
	}
	ev := proto.InputDispatchKeyEvent{
		Modifiers:             mods,
		Text:                  string(r),
		UnmodifiedText:        string(toLowerRU(r)),
		Code:                  k.code,
		Key:                   string(r),
		WindowsVirtualKeyCode: k.vk,
		NativeVirtualKeyCode:  k.vk,
	}

	down := ev
	down.Type = proto.InputDispatchKeyEventTypeKeyDown
	if err := down.Call(page); err != nil {
		return err
	}
	up := ev
	up.Type = proto.InputDispatchKeyEventTypeKeyUp
	up.Text = "" // keyUp текста не несёт
	up.UnmodifiedText = ""
	return up.Call(page)
}
