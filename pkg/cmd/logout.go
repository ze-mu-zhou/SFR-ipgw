package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var (
	LogoutCommand = &cli.Command{
		Name:   "logout",
		Usage:  "logout ipgw",
		Before: rejectPositionalArguments,
		Action: func(ctx *cli.Context) error {
			h := handler.NewIpgwHandler()
			connected, loggedIn, err := h.CheckConnection()
			if err != nil {
				return err
			}
			if !connected {
				return errors.New("当前不在校园网内")
			}
			if !loggedIn {
				return errors.New("尚未登录")
			}
			info := h.GetInfo()
			if err := h.Logout(); err != nil {
				return fmt.Errorf("注销账号 '%s' 失败：\n\t%v", info.Username, err)
			}
			console.InfoF("账号 '%s' 已注销\n", info.Username)
			return nil
		},
		OnUsageError: onUsageError,
	}
)
