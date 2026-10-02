package cmd

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var LogoutCommand = &cli.Command{
	Name:   "logout",
	Usage:  "注销当前登录的校园网账号",
	Before: rejectPositionalArguments,
	Action: func(ctx *cli.Context) error {
		gateway := handler.NewIPGWHandler()
		connected, loggedIn, err := gateway.CheckConnection()
		if err != nil {
			return err
		}
		if !connected {
			return errors.New("当前不在校园网内")
		}
		if !loggedIn {
			return errors.New("尚未登录")
		}
		info := gateway.Info()
		if err := gateway.Logout(); err != nil {
			return fmt.Errorf("注销账号 '%s' 失败：%w", info.Username, err)
		}
		console.Infof("账号 '%s' 已注销\n", info.Username)
		return nil
	},
	OnUsageError: onUsageError,
}
