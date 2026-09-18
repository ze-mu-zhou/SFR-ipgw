package cmd

import (
	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var (
	TestCommand = &cli.Command{
		Name:   "test",
		Usage:  "test whether is connected to the campus network and whether has logged in ipgw",
		Before: rejectPositionalArguments,
		Action: func(ctx *cli.Context) error {
			h := handler.NewIpgwHandler()
			connected, loggedIn, err := h.CheckConnection()
			if err != nil {
				return err
			}
			console.Info("校园网连接： ")
			if connected {
				console.InfoL("已连接")
			} else {
				console.InfoL("未连接")
			}
			console.Info("ipgw 登录：  ")
			if loggedIn {
				console.InfoL("是")
			} else {
				console.InfoL("否")
			}
			return nil
		},
		OnUsageError: onUsageError,
	}
)
