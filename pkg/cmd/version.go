package cmd

import (
	"github.com/ze-mu-zhou/SFR-ipgw"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/urfave/cli/v2"
)

var (
	VersionCommand = &cli.Command{
		Name:  "version",
		Usage: "show version and build info",
		Action: func(ctx *cli.Context) error {
			console.InfoF("ipgw %s\nbuild: %s\ncommit: %s\nkind: %s\nsource: %s\n", ipgw.Version, ipgw.Build, ipgw.Commit, ipgw.BuildKind, ipgw.Repo)
			if ipgw.UpdateEnabled == "true" && ipgw.BuildKind == "release" && ipgw.ReleaseRepo != "" {
				console.InfoF("update source: %s\n", ipgw.ReleaseRepo)
			} else {
				console.InfoL("self-update: disabled (local/custom build)")
			}
			return nil
		},
	}
)
