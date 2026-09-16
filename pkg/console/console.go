package console

import (
	"fmt"
	"os"
)

func FatalL(msg ...interface{}) {
	_, _ = fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}

func Info(msg ...interface{}) {
	fmt.Print(msg...)
}

func InfoL(msg ...interface{}) {
	fmt.Println(msg...)
}

func InfoF(format string, msg ...interface{}) {
	fmt.Printf(format, msg...)
}
