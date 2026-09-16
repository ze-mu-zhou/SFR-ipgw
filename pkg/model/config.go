package model

import (
	"fmt"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
)

type Config struct {
	DefaultAccount string     `json:"default_account"`
	Accounts       []*Account `json:"accounts"`
}

func (c *Config) AddAccount(username, password, secret string) error {
	for _, account := range c.Accounts {
		if account.Username == username {
			return fmt.Errorf("账号 %s 已存在", username)
		}
	}
	a := &Account{Username: username}
	if password != "" {
		if err := a.SetPassword(password, nil); err != nil {
			return err
		}
	}
	c.Accounts = append(c.Accounts, a)
	return nil
}

func (c *Config) GetAccount(username string) *Account {
	for _, account := range c.Accounts {
		if account.Username == username {
			return account
		}
	}
	return nil
}

// DelAccount 删除账号并清理其系统凭据。凭据清理失败时账号仍会被移除，
// 由返回的 error 提示调用方。
func (c *Config) DelAccount(username string) error {
	if c.DefaultAccount == username {
		c.DefaultAccount = ""
	}
	for i, account := range c.Accounts {
		if account.Username == username {
			c.Accounts = append(c.Accounts[:i], c.Accounts[i+1:]...)
			if account.CredentialRef != "" {
				if e := credential.Delete(account.CredentialRef); e != nil {
					return fmt.Errorf("账号已删除，但清理系统凭据失败：%w", e)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("未找到账号 '%s'", username)
}

func (c *Config) SetDefaultAccount(username string) bool {
	for _, account := range c.Accounts {
		if account.Username == username {
			c.DefaultAccount = username
			return true
		}
	}
	return false
}

func (c *Config) GetDefaultAccount() *Account {
	if c.DefaultAccount != "" {
		return c.GetAccount(c.DefaultAccount)
	}
	if len(c.Accounts) > 0 {
		return c.Accounts[0]
	}
	return nil
}
