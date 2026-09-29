package cmd

import (
	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
)

var VersionCommand = &cli.Command{
	Name:   "version",
	Usage:  "显示版本和构建信息",
	Before: rejectPositionalArguments,
	Action: func(ctx *cli.Context) error {
		console.Infof("ipgw %s\n构建：%s\n提交：%s\n类型：%s\n仓库：%s\n", ipgw.Version, ipgw.Build, ipgw.Commit, ipgw.BuildKind, ipgw.Repo)
		if ipgw.UpdateEnabled == "true" && ipgw.BuildKind == "release" && ipgw.ReleaseRepo != "" {
			console.Infof("更新来源：%s\n", ipgw.ReleaseRepo)
		} else {
			console.Infoln("自动更新：已禁用（本地/自定义构建）")
		}
		return nil
	},
}
