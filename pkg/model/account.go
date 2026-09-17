package model

import (
	"errors"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
)

type Account struct {
	Username      string `json:"username"`
	Password      string `json:"-"`
	CredentialRef string `json:"credential_ref,omitempty"`
}

func (a *Account) GetPassword() (string, error) {
	if a.Password != "" {
		return a.Password, nil
	}
	if a.CredentialRef != "" {
		return credential.Load(a.CredentialRef)
	}
	return "", errors.New("没有已保存的密码，请交互输入密码")
}

// SetPassword 将密码存入系统凭据管理器。
// 旧凭据由存储层在配置持久化成功后清理，不能在此提前删除。
func (a *Account) SetPassword(password string) error {
	ref, err := credential.NewReference(a.Username)
	if err != nil {
		return err
	}
	if e := credential.Save(ref, a.Username, password); e != nil {
		return e
	}
	a.CredentialRef = ref
	a.Password = password
	return nil
}
func (a *Account) String() string { return a.Username }
