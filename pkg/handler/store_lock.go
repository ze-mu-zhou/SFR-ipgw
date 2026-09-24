package handler

import (
	"fmt"
	"os"
)

// Lock a stable sidecar, not the configuration itself: Persist replaces the
// configuration by rename. Never remove the sidecar on unlock, otherwise two
// processes could lock different files at the same path. Closing the handle
// (including process termination) releases the operating-system lock.
func lockConfig(path string) (*os.File, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("打开配置锁失败：%w", err)
	}
	if err := lockConfigFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("无法锁定配置，可能正被其他进程修改，请稍后重试：%w", err)
	}
	return f, nil
}
