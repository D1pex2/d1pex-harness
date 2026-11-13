package main

// Сигналов между процессами в Windows нет: syscall.Kill не существует, а
// os.Process.Signal умеет только Kill, то есть TerminateProcess — после него
// start не успел бы ни закрыть браузер, ни убрать файл сессии. Поэтому start
// заводит именованное событие, а stop его взводит: сессия гаснет тем же
// штатным путём, что и по Ctrl-C.
//
// Заодно stop физически не может задеть чужой процесс: события с таким именем
// нет ни у кого, кроме rodkit start.

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// stillActive — код «ещё работает» из GetExitCodeProcess (STILL_ACTIVE).
const stillActive = 259

func stopEvent(pid int) *uint16 {
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`Local\rodkit-stop-%d`, pid))
	if err != nil {
		fatal("имя события остановки: %v", err)
	}
	return name
}

func listenStop(stop chan<- os.Signal) {
	h, err := windows.CreateEvent(nil, 0, 0, stopEvent(os.Getpid()))
	if err != nil {
		fatal("событие остановки: %v", err)
	}
	go func() {
		if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err == nil {
			stop <- syscall.SIGTERM
		}
	}()
}

func requestStop(pid int) error {
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, stopEvent(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetEvent(h)
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Процесс есть, но чужой — например, запущенный от администратора.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == stillActive
}
