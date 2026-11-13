//go:build !windows

package browser

import "os"

// reraiseInterrupt отдаёт Ctrl-C штатному обработчику: подписка уже снята, и
// повторный сигнал завершит процесс так, как если бы дампа не было.
func reraiseInterrupt() {
	if self, err := os.FindProcess(os.Getpid()); err == nil {
		_ = self.Signal(os.Interrupt)
	}
}
