package main

import (
	"os"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/cmd"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
)

func main() {
	if err := cmd.Run(os.Args); err != nil {
		console.Fatalln(cmd.FormatError(err))
	}
}
