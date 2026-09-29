//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package handler

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockConfigFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
