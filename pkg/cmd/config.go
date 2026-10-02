package cmd

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

var ConfigCommand = &cli.Command{
	Name:   "config",
	Usage:  "管理配置",
	Action: showCommandGroupHelp,
	Subcommands: []*cli.Command{
		{
			Name:   "account",
			Usage:  "管理配置中保存的账号",
			Action: showCommandGroupHelp,
			Subcommands: []*cli.Command{
				configAccountAddCommand,
				configAccountDeleteCommand,
				configAccountSetCommand,
				configAccountListCommand,
			},
			OnUsageError: onUsageError,
		},
	},
	OnUsageError: onUsageError,
}

var configAccountAddCommand = &cli.Command{
	Name:   "add",
	Usage:  "添加账号；默认将密码保存到系统凭据管理器或密钥环",
	Before: rejectPositionalArguments,
	Flags: append(credentialFlags(),
		&cli.BoolFlag{Name: "default", Usage: "设为默认账号"},
		&cli.BoolFlag{Name: "no-store-password", Usage: "只保存账号，不保存密码；每次使用时输入"},
	),
	Action: func(ctx *cli.Context) error {
		store, err := loadStore(ctx)
		if err != nil {
			return err
		}
		username := ctx.String("username")
		if username == "" {
			return errors.New("请用 -u 指定学号")
		}
		if store.Config.GetAccount(username) != nil {
			return errors.New("账号已存在，请使用 config account set")
		}
		if ctx.Bool("no-store-password") && ctx.Bool("ask-password") {
			return errors.New("--no-store-password 不能与 --ask-password 同时使用")
		}
		password := ""
		if !ctx.Bool("no-store-password") {
			password, err = promptPassword(ctx)
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
			return fmt.Errorf("添加账号失败：%w", err)
		}
		if warning != nil {
			_, _ = fmt.Fprintf(ctx.App.ErrWriter, "警告：%v\n", warning)
		}
		console.Infof("'%s' 已添加\n", username)
		return nil
	},
	OnUsageError: onUsageError,
}

var configAccountDeleteCommand = &cli.Command{
	Name:   "del",
	Usage:  "从配置中删除账号及其保存的密码",
	Before: rejectPositionalArguments,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "username",
			Aliases:  []string{"u"},
			Required: true,
			Usage:    "要删除的`学号`",
		},
	},
	Action: func(ctx *cli.Context) error {
		store, err := loadStore(ctx)
		if err != nil {
			return err
		}
		username := ctx.String("username")

		warning, err := store.UpdateConfig(func(config *model.Config) error {
			return config.DeleteAccount(username)
		})
		if err != nil {
			return fmt.Errorf("删除账号失败：%w", err)
		}
		if warning != nil {
			_, _ = fmt.Fprintf(ctx.App.ErrWriter, "警告：%v\n", warning)
		}
		console.Infof("'%s' 已删除\n", username)
		return nil
	},
	OnUsageError: onUsageError,
}

var configAccountSetCommand = &cli.Command{
	Name:   "set",
	Usage:  "修改账号密码，或设为默认账号",
	Before: rejectPositionalArguments,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "username", Aliases: []string{"u"}, Required: true, Usage: "要修改的`学号`"},
		&cli.BoolFlag{Name: "ask-password", Usage: "交互输入新密码（与 --default 同时使用时也修改密码）"},
		&cli.BoolFlag{Name: "default", Usage: "设为默认账号"},
	},
	Action: func(ctx *cli.Context) error {
		store, err := loadStore(ctx)
		if err != nil {
			return err
		}
		username := ctx.String("username")
		if store.Config.GetAccount(username) == nil {
			return fmt.Errorf("修改账号失败：%w", fmt.Errorf("未找到 '%s'", username))
		}

		changePassword := !ctx.Bool("default") || ctx.Bool("ask-password")
		var password string
		if changePassword {
			password, err = promptPassword(ctx)
			if err != nil {
				return err
			}
		}
		warning, err := store.UpdateConfig(func(config *model.Config) error {
			// Another process may have deleted the account while we prompted.
			current := config.GetAccount(username)
			if current == nil {
				return fmt.Errorf("未找到 '%s'", username)
			}
			if changePassword {
				if err := current.SetPassword(password); err != nil {
					return fmt.Errorf("设置密码失败：%w", err)
				}
			}
			if ctx.Bool("default") {
				config.SetDefaultAccount(username)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("修改账号失败：%w", err)
		}
		if warning != nil {
			_, _ = fmt.Fprintf(ctx.App.ErrWriter, "警告：%v\n", warning)
		}
		console.Infof("'%s' 已修改\n", username)
		return nil
	},
	OnUsageError: onUsageError,
}

var configAccountListCommand = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "列出配置中的账号",
	Before:  rejectPositionalArguments,
	Action: func(ctx *cli.Context) error {
		store, err := loadStore(ctx)
		if err != nil {
			return err
		}

		for i, account := range store.Config.Accounts {
			console.Infof("#%d %s", i, account.String())
			if account.Username == store.Config.DefaultAccount {
				console.Info(" - 默认")
			}
			console.Infoln()
		}

		return nil
	},
	OnUsageError: onUsageError,
}
