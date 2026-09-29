package cmd

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

var LoginCommand = &cli.Command{
	Name:   "login",
	Usage:  "登录校园网",
	Before: rejectPositionalArguments,
	Flags: append(credentialFlags(), &cli.BoolFlag{
		Name: "info", Aliases: []string{"i"}, Usage: "登录成功后显示账号用量信息",
	}),
	Action: func(ctx *cli.Context) error {
		account, err := accountFromContext(ctx)
		if err != nil {
			return err
		}
		gateway := handler.NewIPGWHandler()
		if err = login(gateway, account); err != nil {
			return fmt.Errorf("登录失败：\n\t%v", err)
		}
		if ctx.Bool("info") {
			if err = gateway.FetchUsageInfo(); err != nil {
				return fmt.Errorf("查询信息失败：\n\t%v", err)
			}
			info := gateway.Info()
			console.Infof("\tIP\t%16s\n\t余额\t%16s\n\t流量\t%16s\n\t时长\t%16s\n",
				info.IP,
				info.FormattedBalance(),
				info.FormattedTraffic(),
				info.FormattedUsedTime())
		}
		return nil
	},
	OnUsageError: onUsageError,
}

func login(gateway *handler.IPGWHandler, account *model.Account) error {
	connected, loggedIn, err := gateway.CheckConnection()
	if err != nil {
		return err
	}
	if !connected {
		return errors.New("当前不在校园网内")
	}
	if loggedIn {
		return fmt.Errorf("已以 '%s' 登录", gateway.Info().Username)
	}
	if err := gateway.Login(account); err != nil {
		return err
	}
	if gateway.Info().Username == "" {
		return errors.New("网关未返回在线账号，原因未知")
	}
	console.Infoln("登录成功")
	return nil
}
