//go:build !windows
// +build !windows

package credential

import "errors"

func Save(ref, username, password string) error {
	return errors.New("密码仅支持保存到 Windows 凭据管理器，请交互输入密码")
}
func Load(ref string) (string, error) {
	return "", errors.New("此平台不支持 Windows 凭据，请交互输入密码")
}

// Delete 是空操作：非 Windows 平台 Save 必然失败，不存在存量条目。
func Delete(ref string) error { return nil }
