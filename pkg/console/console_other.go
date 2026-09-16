//go:build !windows
// +build !windows

package console

import (
	"io"
	"os"
)

func newStdoutWriter() io.Writer { return os.Stdout }
func newStderrWriter() io.Writer { return os.Stderr }
