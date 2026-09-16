package console

import (
	"fmt"
	"io"
	"os"
)

// Stdout、Stderr 是进程标准输出/错误的写入器。在 Windows 控制台中通过
// WriteConsoleW 写入，中文不依赖控制台代码页；重定向时保持 UTF-8 字节。
var (
	Stdout io.Writer = newStdoutWriter()
	Stderr io.Writer = newStderrWriter()
)

func FatalL(msg ...interface{}) {
	_, _ = fmt.Fprintln(Stderr, msg...)
	os.Exit(1)
}

func Info(msg ...interface{}) {
	_, _ = fmt.Fprint(Stdout, msg...)
}

func InfoL(msg ...interface{}) {
	_, _ = fmt.Fprintln(Stdout, msg...)
}

func InfoF(format string, msg ...interface{}) {
	_, _ = fmt.Fprintf(Stdout, format, msg...)
}
