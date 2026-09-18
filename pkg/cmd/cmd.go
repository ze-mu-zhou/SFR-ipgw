package cmd

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

var (
	App = &cli.App{
		Name:      "ipgw",
		HelpName:  "ipgw",
		Copyright: "Home page:\thttps://github.com/ze-mu-zhou/SFR-ipgw\nFeedback:\thttps://github.com/ze-mu-zhou/SFR-ipgw/issues/new",
		Commands: []*cli.Command{
			LoginCommand,
			LogoutCommand,
			KickCommand,
			InfoCommand,
			CompareCommand,
			ConfigCommand,
			TestCommand,
			VersionCommand,
			UpdateCommand,
		},
		Action: func(ctx *cli.Context) error {
			if ctx.NArg() != 0 {
				return errors.New("未知命令或多余参数，请使用 --help 查看用法")
			}
			return loginUseDefaultAccount(ctx)
		},
		HideVersion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"f"},
				Usage:   "load configuration from specific `file`",
			},
		},
		OnUsageError: onUsageError,
		Writer:       console.Stdout,
		ErrWriter:    console.Stderr,
	}
)

func loginUseDefaultAccount(ctx *cli.Context) error {
	account, err := getAccountByContext(ctx)
	if err != nil {
		return err
	}
	console.InfoF("使用账号 '%s'\n", account.Username)

	if err = login(handler.NewIpgwHandler(), account); err != nil {
		return fmt.Errorf("登录失败：\n\t%v", err)
	}
	return nil
}

func getAccountByContext(ctx *cli.Context) (account *model.Account, err error) {
	username := ctx.String("username")
	if username != "" && ctx.Bool("ask-password") {
		password, err := promptPassword(ctx)
		if err != nil {
			return nil, err
		}
		return &model.Account{Username: username, Password: password}, nil
	}
	store, err := getStoreHandler(ctx)
	if err != nil {
		return nil, err
	}
	if username == "" {
		account = store.Config.GetDefaultAccount()
		if account == nil {
			return nil, errors.New("没有默认账号，请用 -u 学号指定账号")
		}
	} else {
		account = store.Config.GetAccount(username)
		if account == nil {
			account = &model.Account{Username: username}
		}
	}
	if ctx.Bool("ask-password") || (account.CredentialRef == "" && account.Password == "") {
		account.Password, err = promptPassword(ctx)
		if err != nil {
			return nil, err
		}
	}
	return account, nil
}

func getStoreHandler(ctx *cli.Context) (store *handler.StoreHandler, err error) {
	if store, err = handler.NewStoreHandler(ctx.String("config")); err == nil {
		err = store.Load()
	}
	return
}

func onUsageError(ctx *cli.Context, err error, isSubcommand bool) error {
	_, _ = fmt.Fprintf(ctx.App.Writer, "%s\n\n", err.Error())
	if isSubcommand {
		cli.ShowSubcommandHelpAndExit(ctx, 1)
	} else {
		cli.ShowAppHelpAndExit(ctx, 1)
	}
	return nil
}

func init() {
	cli.AppHelpTemplate = `USAGE:
   {{.HelpName}} {{if .VisibleFlags}}[global options]{{end}}{{if .Commands}} command [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}
{{if .Commands}}
COMMANDS:
{{range .Commands}}{{if not .HideHelp}}   {{join .Names ", "}}{{ "\t"}}{{.Usage}}{{ "\n" }}{{end}}{{end}}{{end}}
OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}
{{.Copyright}}
`
	cli.CommandHelpTemplate = `{{.Usage}}

USAGE:
   {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}}{{if .VisibleFlags}} [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{end}}{{if .Category}}

CATEGORY:
   {{.Category}}{{end}}{{if .Description}}

DESCRIPTION:
   {{.Description | nindent 3 | trim}}{{end}}{{if .VisibleFlags}}

OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
`

	cli.SubcommandHelpTemplate = `{{.Usage}}

USAGE:
   {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} command{{if .VisibleFlags}} [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{end}}{{if .Description}}

DESCRIPTION:
   {{.Description | nindent 3 | trim}}{{end}}

COMMANDS:{{range .VisibleCategories}}{{if .Name}}
   {{.Name}}:{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{else}}{{range .VisibleCommands}}
   {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{if .VisibleFlags}}

OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
`
	return
}
