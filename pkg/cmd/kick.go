package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var (
	KickCommand = &cli.Command{
		Name:                   "kick",
		Usage:                  "logout any specific device by SID",
		ArgsUsage:              "[sid list]",
		UseShortOptionHandling: true,
		Flags:                  credentialFlags(),
		Before:                 validateKickArguments,
		Action: func(ctx *cli.Context) error {
			sids := ctx.Args().Slice()
			account, err := getAccountByContext(ctx)
			if err != nil {
				return err
			}
			h := handler.NewIpgwHandler()
			password, err := account.GetPassword()
			if err != nil {
				return err
			}
			if err = h.NEUAuth(account.Username, password); err != nil {
				return err
			}
			failed := 0
			for _, sid := range sids {
				result, err := h.Kick(sid)
				if result {
					console.InfoF("#%s: 成功\n", sid)
				} else {
					failed++
					console.InfoF("#%s: 失败\n", sid)
					if err != nil {
						console.InfoF("\t%v\n", err)
					}
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d 个设备下线失败", failed)
			}
			return nil
		},
	}
)
