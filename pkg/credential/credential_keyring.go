package credential

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/zalando/go-keyring"
)

// service 是系统密钥环中的服务名；账号条目以凭据引用区分。
const service = "ipgw"

// Save 将密码写入系统凭据存储（Windows 凭据管理器、macOS 钥匙串或 Secret Service）。
func Save(ref, username, password string) error {
	if ref == "" || username == "" {
		return errors.New("无效的凭据引用或账号名")
	}
	if err := keyring.Set(service, ref, password); err != nil {
		return keyringError("写入", err)
	}
	return nil
}

func Load(ref string) (string, error) {
	password, err := keyring.Get(service, ref)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", errors.New("系统凭据存储中没有已保存的密码，请重新输入密码")
	}
	if err != nil {
		return "", keyringError("读取", err)
	}
	return password, nil
}

// Delete 从系统凭据存储删除条目；条目不存在时视为成功（幂等）。
func Delete(ref string) error {
	err := keyring.Delete(service, ref)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return keyringError("删除", err)
}

func keyringError(action string, err error) error {
	switch {
	case errors.Is(err, keyring.ErrUnsupportedPlatform):
		return errors.New("此平台不支持系统密钥环，无法保存密码；请交互输入密码，或添加账号时使用 --no-store-password")
	case errors.Is(err, keyring.ErrSetDataTooBig):
		return errors.New("密码长度超出系统凭据存储的支持范围")
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("%sWindows 凭据管理器失败：%w", action, err)
	}
	return fmt.Errorf("%s系统密钥环失败（Linux 需要运行 Secret Service，如 GNOME Keyring 或 KWallet）：%w", action, err)
}
