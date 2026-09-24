//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package handler

import (
	"errors"
	"os"
)

func lockConfigFile(_ *os.File) error {
	return errors.New("此平台暂不支持安全的配置文件锁")
}
