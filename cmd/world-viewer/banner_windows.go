//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// 旧版 Windows 控制台默认不解释 ANSI 转义，需要先打开虚拟终端处理。
func enableTerminalColors() bool {
	const virtualTerminalProcessing = 0x0004
	kernel := syscall.NewLazyDLL("kernel32.dll")
	handle := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	if ok, _, _ := kernel.NewProc("GetConsoleMode").Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); ok == 0 {
		return false
	}
	ok, _, _ := kernel.NewProc("SetConsoleMode").Call(uintptr(handle), uintptr(mode|virtualTerminalProcessing))
	return ok != 0
}
