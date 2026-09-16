//go:build windows
// +build windows

package console

import (
	"io"
	"os"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var (
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procWriteConsoleW  = kernel32.NewProc("WriteConsoleW")
)

func newStdoutWriter() io.Writer { return consoleWriter{file: os.Stdout} }
func newStderrWriter() io.Writer { return consoleWriter{file: os.Stderr} }

// consoleWriter 通过 WriteConsoleW（UTF-16）向 Windows 控制台写入 UTF-8 文本，
// 使中文输出不依赖控制台代码页（默认 GBK）。句柄被重定向到管道或文件时
// 透传原始字节，保持 UTF-8 输出。
type consoleWriter struct{ file *os.File }

func (w consoleWriter) Write(p []byte) (int, error) {
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(w.file.Fd(), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return w.file.Write(p)
	}
	text := utf16.Encode([]rune(string(p)))
	for len(text) > 0 {
		chunk := text
		if len(chunk) > 16000 {
			chunk = chunk[:16000]
		}
		var written uint32
		if r, _, err := procWriteConsoleW.Call(w.file.Fd(), uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk)), uintptr(unsafe.Pointer(&written)), 0); r == 0 {
			return 0, err
		}
		text = text[written:]
	}
	return len(p), nil
}
