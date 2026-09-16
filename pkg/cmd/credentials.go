package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

func credentialFlags(cookie bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "username", Aliases: []string{"u"}, Usage: "student number (uses stored default when omitted)"},
		&cli.StringFlag{Name: "password", Aliases: []string{"p"}, Usage: "password; omit for hidden terminal input or stored credentials"},
		&cli.BoolFlag{Name: "ask-password", Usage: "prompt for password instead of using a saved credential"},
		&cli.StringFlag{Name: "secret", Aliases: []string{"s"}, Usage: "secret for a legacy encrypted account"},
	}
	if cookie {
		flags = append(flags, &cli.StringFlag{Name: "cookie", Aliases: []string{"c"}, Usage: "existing campus gateway session cookie"})
	}
	return flags
}

func promptPassword(ctx *cli.Context) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("缺少密码：请在交互终端运行以隐藏输入密码，或使用已保存的凭据")
	}
	fmt.Fprint(ctx.App.ErrWriter, "统一身份认证密码（输入不显示）：")
	value, err := term.ReadPassword(fd)
	fmt.Fprintln(ctx.App.ErrWriter)
	if err != nil {
		return "", fmt.Errorf("读取密码失败：%w", err)
	}
	defer func() {
		for i := range value {
			value[i] = 0
		}
	}()
	if len(value) == 0 {
		return "", errors.New("密码不能为空")
	}
	return string(value), nil
}

func suppliedOrPromptPassword(ctx *cli.Context) (string, error) {
	if ctx.Bool("ask-password") && ctx.IsSet("password") {
		return "", errors.New("--ask-password 与 --password 不能同时使用")
	}
	if ctx.IsSet("password") {
		if ctx.String("password") == "" {
			return "", errors.New("密码不能为空")
		}
		return ctx.String("password"), nil
	}
	return promptPassword(ctx)
}
