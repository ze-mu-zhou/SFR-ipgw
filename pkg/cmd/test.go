package cmd

import (
	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var TestCommand = &cli.Command{
	Name:   "test",
	Usage:  "检测是否连接校园网，以及是否已登录网关",
	Before: rejectPositionalArguments,
	Action: func(ctx *cli.Context) error {
		connected, loggedIn, err := handler.NewIPGWHandler().CheckConnection()
		if err != nil {
			return err
		}
		console.Info("校园网连接： ")
		if connected {
			console.Infoln("已连接")
		} else {
			console.Infoln("未连接")
		}
		console.Info("ipgw 登录：  ")
		if loggedIn {
			console.Infoln("是")
		} else {
			console.Infoln("否")
		}
		return nil
	},
	OnUsageError: onUsageError,
}
