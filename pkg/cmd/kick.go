package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var KickCommand = &cli.Command{
	Name:                   "kick",
	Usage:                  "按 SID 让指定设备下线",
	ArgsUsage:              "[SID 列表]",
	UseShortOptionHandling: true,
	Flags:                  credentialFlags(),
	Before:                 validateKickArguments,
	Action: func(ctx *cli.Context) error {
		sids := ctx.Args().Slice()
		account, err := accountFromContext(ctx)
		if err != nil {
			return err
		}
		gateway := handler.NewIPGWHandler()
		password, err := account.GetPassword()
		if err != nil {
			return err
		}
		if err = gateway.NEUAuth(account.Username, password); err != nil {
			return err
		}
		failed := 0
		for _, sid := range sids {
			kicked, err := gateway.Kick(sid)
			if kicked {
				console.Infof("#%s: 成功\n", sid)
				continue
			}
			failed++
			_, _ = fmt.Fprintf(ctx.App.ErrWriter, "#%s: 失败\n", sid)
			if err != nil {
				_, _ = fmt.Fprintf(ctx.App.ErrWriter, "\t%v\n", err)
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d 个设备下线失败", failed)
		}
		return nil
	},
	OnUsageError: onUsageError,
}
