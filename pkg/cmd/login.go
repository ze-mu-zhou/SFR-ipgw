package cmd

import (
	"errors"
	"fmt"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"github.com/urfave/cli/v2"
)

var (
	LoginCommand = &cli.Command{
		Name:  "login",
		Usage: "login ipgw",
		Flags: append(credentialFlags(true), &cli.BoolFlag{
			Name: "info", Aliases: []string{"i"}, Usage: "output account info after login successfully",
		}),
		Action: func(ctx *cli.Context) error {
			account, err := getAccountByContext(ctx)
			if err != nil {
				return err
			}
			h := handler.NewIpgwHandler()
			if err = login(h, account); err != nil {
				return fmt.Errorf("登录失败：\n\t%v", err)
			}
			if ctx.Bool("info") {
				if err = h.FetchUsageInfo(); err != nil {
					return fmt.Errorf("查询信息失败：\n\t%v", err)
				}
				info := h.GetInfo()
				console.InfoF("\tIP\t%16s\n\t余额\t%16s\n\t流量\t%16s\n\t时长\t%16s\n",
					info.IP,
					info.FormattedBalance(),
					info.FormattedTraffic(),
					info.FormattedUsedTime())
			}
			return nil
		},
		OnUsageError: onUsageError,
	}
)

func login(h *handler.IpgwHandler, account *model.Account) error {
	// check logged
	connected, loggedIn, err := h.CheckConnection()
	if err != nil {
		return err
	}
	if !connected {
		return errors.New("当前不在校园网内")
	}
	if loggedIn {
		return fmt.Errorf("已以 '%s' 登录", h.GetInfo().Username)
	}
	if err := h.Login(account); err != nil {
		return err
	}
	info := h.GetInfo()
	if info.Username == "" {
		return fmt.Errorf("未知原因")
	}
	console.InfoL("登录成功")
	return nil
}
