package browser

import "os"

// statusControlCExit — STATUS_CONTROL_C_EXIT, код выхода процесса, которого
// штатно погасил Ctrl-C.
var statusControlCExit uint32 = 0xC000013A

// reraiseInterrupt делает то, что сделал бы штатный обработчик Ctrl-C.
//
// Переслать сигнал себе, как на unix, в Windows нечем: Process.Signal умеет
// только Kill, а GenerateConsoleCtrlEvent бьёт по всей консоли, включая
// оболочку. Без выхода здесь прерванный go test не завершался бы вовсе — ждал
// второго Ctrl-C или таймаута.
func reraiseInterrupt() {
	os.Exit(int(statusControlCExit))
}
