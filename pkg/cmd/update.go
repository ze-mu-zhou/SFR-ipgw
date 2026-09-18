package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

var (
	UpdateCommand = &cli.Command{
		Name:   "update",
		Usage:  "check latest version of ipgw and update",
		Before: rejectPositionalArguments,
		Action: func(ctx *cli.Context) error {
			h := handler.NewUpdateHandler()
			newer, err := h.CheckLatestVersion()
			if err != nil {
				return err
			}
			if !newer {
				console.InfoL("已是最新版本")
				return nil
			}
			err = h.Update()
			if err != nil {
				return fmt.Errorf("更新失败：\n\t%v", err)
			}
			console.InfoL("更新成功")
			return nil
		},
	}
)
