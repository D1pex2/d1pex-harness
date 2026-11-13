//go:build !windows

package main

import (
	"os"
	"syscall"
)

// listenStop здесь пуст: rodkit stop шлёт SIGTERM, а его start и так ловит.
func listenStop(chan<- os.Signal) {}

func requestStop(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }

func alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }
