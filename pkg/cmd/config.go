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
					configAccountMigrateCommand,
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
				if err := config.AddAccount(username, password, ctx.String("secret")); err != nil {
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

			if ctx.IsSet("secret") {
				return fmt.Errorf("更新系统凭据不使用 --secret；旧配置请执行 config account migrate")
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
					if err := config.GetAccount(username).SetPassword(password, nil); err != nil {
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

var configAccountMigrateCommand = &cli.Command{
	Name: "migrate", Usage: "explicitly migrate legacy encrypted passwords to the Windows credential vault",
	Flags: []cli.Flag{&cli.StringFlag{Name: "secret", Aliases: []string{"s"}, Usage: "legacy encryption secret (empty by default)"}},
	Action: func(ctx *cli.Context) error {
		store, err := getStoreHandler(ctx)
		if err != nil {
			return err
		}
		count := 0
		for _, a := range store.Config.Accounts {
			if a.EncryptedPassword != "" {
				count++
			}
		}
		if count == 0 {
			console.InfoL("没有需要迁移的旧密码")
			return nil
		}
		if err = store.MigrateCredentials(ctx.String("secret")); err != nil {
			return err
		}
		console.InfoF("已迁移 %d 个账号的密码到系统凭据管理器\n", count)
		return nil
	},
	OnUsageError: onUsageError,
}
