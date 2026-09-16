package model

import (
	"errors"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/utils"
)

type Account struct {
	Username          string `json:"username"`
	Password          string `json:"-"`
	Cookie            string `json:"-"`
	Secret            string `json:"-"`
	EncryptedPassword string `json:"encrypted_password,omitempty"`
	CredentialRef     string `json:"credential_ref,omitempty"`
}

func (a *Account) GetPassword() (string, error) {
	if a.Password != "" {
		return a.Password, nil
	}
	if a.CredentialRef != "" {
		return credential.Load(a.CredentialRef)
	}
	if a.EncryptedPassword == "" {
		return "", errors.New("没有已保存的密码，请交互输入密码")
	}
	p, e := utils.Decrypt(a.EncryptedPassword, []byte(a.Secret))
	if e != nil {
		return "", errors.New("旧密码解密失败，请检查 secret")
	}
	return p, nil
}

// SetPassword 将密码存入系统凭据管理器；secret 仅为兼容旧配置保留。
// 保存成功后删除旧 CredentialRef 对应的条目（best-effort，失败不阻断）。
func (a *Account) SetPassword(password string, secret []byte) error {
	ref, err := credential.NewReference(a.Username)
	if err != nil {
		return err
	}
	if e := credential.Save(ref, a.Username, password); e != nil {
		return e
	}
	if a.CredentialRef != "" && a.CredentialRef != ref {
		_ = credential.Delete(a.CredentialRef)
	}
	a.CredentialRef = ref
	a.Password = password
	a.EncryptedPassword = ""
	return nil
}
func (a *Account) String() string { return a.Username }
