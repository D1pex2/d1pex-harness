package browser

// Живой ввод для JS-поиска rod. Вся механика — в пакете human, тут только
// добыча прямоугольника из найденного элемента.

import (
	"context"

	"github.com/go-rod/rod"

	"rodkit/human"
)

// humanClick подводит курсор дугой, выжидает и жмёт мимо центра.
//
// ctx обязателен: у страницы, добытой через el.Page(), контекст свой — тот, что
// Tab.Element гасит своим CancelTimeout сразу после поиска. Мышь и клавиатура
// живут на странице, поэтому без подмены контекста любой ввод длиннее пары
// сотен миллисекунд падал с «context canceled».
func humanClick(ctx context.Context, el *rod.Element) error {
	if err := el.ScrollIntoView(); err != nil {
		return err
	}
	shape, err := el.Shape()
	if err != nil {
		return err
	}
	return human.ClickAt(el.Page().Context(ctx), shape.Box())
}

// humanType кликает в поле и печатает посимвольно настоящими key-событиями.
func humanType(ctx context.Context, el *rod.Element, text string) error {
	if err := humanClick(ctx, el); err != nil {
		return err
	}
	// Чистим старое значение через select+insertText: событий клавиатуры это не
	// порождает, а Ctrl+A выставил бы hasModifierKeyPressed.
	if err := el.SelectAllText(); err == nil {
		_ = el.Input("")
	}
	return human.TypeText(el.Page().Context(ctx), text)
}
