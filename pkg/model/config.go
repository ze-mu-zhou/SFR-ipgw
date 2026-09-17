package model

import "fmt"

type Config struct {
	DefaultAccount string     `json:"default_account"`
	Accounts       []*Account `json:"accounts"`
}

func (c *Config) AddAccount(username, password string) error {
	for _, account := range c.Accounts {
		if account.Username == username {
			return fmt.Errorf("账号 %s 已存在", username)
		}
	}
	a := &Account{Username: username}
	if password != "" {
		if err := a.SetPassword(password); err != nil {
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

// DelAccount 仅修改配置；系统凭据由存储层在持久化成功后清理。
func (c *Config) DelAccount(username string) error {
	for i, account := range c.Accounts {
		if account.Username == username {
			c.Accounts = append(c.Accounts[:i], c.Accounts[i+1:]...)
			if c.DefaultAccount == username {
				c.DefaultAccount = ""
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
