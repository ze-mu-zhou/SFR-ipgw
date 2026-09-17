package cmd

import (
	"fmt"
	"runtime"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

var (
	ConfigCommand = &cli.Command{
		Name:  "config",
		Usage: "manage config",
		Subcommands: []*cli.Command{
			{
				Name:  "account",
				Usage: "manage accounts stored in config",
				Subcommands: []*cli.Command{
					configAccountAddCommand,
					configAccountDelCommand,
					configAccountSetCommand,
					configAccountListCommand,
				},
				OnUsageError: onUsageError,
			},
		},
		OnUsageError: onUsageError,
	}

	configAccountAddCommand = &cli.Command{
		Name:  "add",
		Usage: "add account into config",
		Flags: append(credentialFlags(false),
			&cli.BoolFlag{Name: "default", Usage: "set as default account"},
			&cli.BoolFlag{Name: "no-store-password", Usage: "save account name only; prompt each time"},
		),
		Action: func(ctx *cli.Context) error {
			store, err := getStoreHandler(ctx)
			if err != nil {
				return err
			}
			username := ctx.String("username")
			if username == "" {
				return fmt.Errorf("请用 -u 指定学号")
			}
			if store.Config.GetAccount(username) != nil {
				return fmt.Errorf("账号已存在，请使用 config account set")
			}
			password := ""
			if runtime.GOOS != "windows" && (ctx.IsSet("password") || ctx.Bool("ask-password")) {
				return fmt.Errorf("此平台不保存密码；请仅保存账号，查询时在终端输入密码")
			}
			if ctx.Bool("no-store-password") && (ctx.IsSet("password") || ctx.Bool("ask-password")) {
				return fmt.Errorf("--no-store-password 不能与密码参数同时使用")
			}
			if !ctx.Bool("no-store-password") && runtime.GOOS == "windows" {
				password, err = suppliedOrPromptPassword(ctx)
				if err != nil {
					return err
				}
			}
			warning, err := store.UpdateConfig(func(config *model.Config) error {
				if err := config.AddAccount(username, password); err != nil {
					return err
				}
				if ctx.Bool("default") {
					config.SetDefaultAccount(username)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("添加账号失败：\n\t%v", err)
			}
			if warning != nil {
				_, _ = fmt.Fprintf(ctx.App.ErrWriter, "警告：%v\n", warning)
			}
			console.InfoF("'%s' 已添加\n", username)
			return nil
		},
		OnUsageError: onUsageError,
	}

	configAccountDelCommand = &cli.Command{
		Name:  "del",
		Usage: "delete account from config",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "username",
				Aliases:  []string{"u"},
				Required: true,
				Usage:    "student number `id` to be deleted",
			},
		},
		Action: func(ctx *cli.Context) error {
			store, err := getStoreHandler(ctx)
			if err != nil {
				return err
			}
			username := ctx.String("username")

			warning, err := store.UpdateConfig(func(config *model.Config) error {
				return config.DelAccount(username)
			})
			if err != nil {
				return fmt.Errorf("删除账号失败：\n\t%v", err)
			}
			if warning != nil {
				_, _ = fmt.Fprintf(ctx.App.ErrWriter, "警告：%v\n", warning)
			}
			console.InfoF("'%s' 已删除\n", username)
			return nil
		},
		OnUsageError: onUsageError,
	}

	configAccountSetCommand = &cli.Command{
		Name:  "set",
		Usage: "edit account in config",
		Flags: append(credentialFlags(false), &cli.BoolFlag{Name: "default", Usage: "set as default account"}),
		Action: func(ctx *cli.Context) error {
			store, err := getStoreHandler(ctx)
			if err != nil {
				return err
			}
			username := ctx.String("username")
			account := store.Config.GetAccount(username)
			if account == nil {
				return fmt.Errorf("修改账号失败：\n\t未找到 '%s'", username)
			}

			changePassword := !ctx.Bool("default") || ctx.IsSet("password") || ctx.Bool("ask-password")
			var password string
			if changePassword {
				if runtime.GOOS != "windows" {
					return fmt.Errorf("此平台不保存密码；请在查询时输入密码")
				}
				password, err = suppliedOrPromptPassword(ctx)
				if err != nil {
					return err
				}
			}
			warning, err := store.UpdateConfig(func(config *model.Config) error {
				if changePassword {
					if err := config.GetAccount(username).SetPassword(password); err != nil {
						return fmt.Errorf("设置密码失败：\n\t%v", err)
					}
				}
				if ctx.Bool("default") {
					config.SetDefaultAccount(username)
				}
				return nil
			})
			if err != nil {
				return err
			}
			if warning != nil {
				_, _ = fmt.Fprintf(ctx.App.ErrWriter, "警告：%v\n", warning)
			}
			console.InfoF("'%s' 已修改\n", username)
			return nil
		},
		OnUsageError: onUsageError,
	}

	configAccountListCommand = &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "list accounts in config",
		Action: func(ctx *cli.Context) error {
			store, err := getStoreHandler(ctx)
			if err != nil {
				return err
			}

			for i, account := range store.Config.Accounts {
				console.InfoF("#%d %s", i, account.String())
				if account.Username == store.Config.DefaultAccount {
					console.Info(" - 默认")
				}
				console.InfoL()
			}

			return nil
		},
		OnUsageError: onUsageError,
	}
)

