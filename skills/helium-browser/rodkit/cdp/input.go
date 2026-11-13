package cdp

import (
	"fmt"

	"github.com/go-rod/rod/lib/proto"

	"rodkit/human"
)

// Click подводит курсор к узлу дугой и жмёт мимо центра. Никакого
// element.click(): событие рождается в Input.dispatchMouseEvent и приходит на
// страницу с isTrusted: true, неотличимо от настоящей мыши.
func (t *Tab) Click(id proto.DOMBackendNodeID) error {
	if err := t.ScrollTo(id); err != nil {
		return err
	}
	box, err := t.Box(id)
	if err != nil {
		return fmt.Errorf("клик: %w", err)
	}
	return human.ClickAt(t.page, box)
}

// Type кликает в поле, чистит старое значение и печатает посимвольно. Клик
// обязателен: детектор отдельно смотрит, был ли по полю клик вообще
// (<поле>FieldHasClick), — заполненное без касания поле выглядит как скрипт.
func (t *Tab) Type(id proto.DOMBackendNodeID, text string) error {
	if err := t.Click(id); err != nil {
		return err
	}
	if err := human.ClearField(t.page); err != nil {
		return err
	}
	return human.TypeText(t.page, text)
}

// Focus ставит фокус без клика — для случаев, когда поле уже под курсором.
func (t *Tab) Focus(id proto.DOMBackendNodeID) error {
	return proto.DOMFocus{BackendNodeID: id}.Call(t.page)
}
