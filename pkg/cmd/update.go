package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var UpdateCommand = &cli.Command{
	Name:   "update",
	Usage:  "检查最新版本并更新 ipgw",
	Before: rejectPositionalArguments,
	Action: func(ctx *cli.Context) error {
		updater := handler.NewUpdateHandler()
		newer, err := updater.CheckLatestVersion()
		if err != nil {
			return err
		}
		if !newer {
			console.Infoln("已是最新版本")
			return nil
		}
		if err = updater.Update(); err != nil {
			return fmt.Errorf("更新失败：%w", err)
		}
		console.Infoln("更新成功")
		return nil
	},
	OnUsageError: onUsageError,
}
