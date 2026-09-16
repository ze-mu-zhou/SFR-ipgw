package main

import (
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/cmd"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"os"
)

func main() {
	if err := cmd.App.Run(os.Args); err != nil {
		console.FatalL(err.Error())
	}
}
