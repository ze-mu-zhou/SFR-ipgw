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
				return fmt.Errorf("login failed: \n\t%v", err)
			}
			if ctx.Bool("info") {
				if err = h.FetchUsageInfo(); err != nil {
					return fmt.Errorf("fetch info failed: \n\t%v", err)
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
		return errors.New("not in campus network")
	}
	if loggedIn {
		return fmt.Errorf("already logged in as '%s'", h.GetInfo().Username)
	}
	if err := h.Login(account); err != nil {
		return err
	}
	info := h.GetInfo()
	if info.Username == "" {
		return fmt.Errorf("unknown reason")
	}
	console.InfoL("login successfully")
	return nil
}
