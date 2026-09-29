package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

func credentialFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "username", Aliases: []string{"u"}, Usage: "`学号`（省略时使用配置中的默认账号）"},
		&cli.BoolFlag{Name: "ask-password", Usage: "交互输入密码，不使用已保存的密码"},
	}
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
