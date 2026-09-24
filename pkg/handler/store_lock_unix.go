//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package handler

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockConfigFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
