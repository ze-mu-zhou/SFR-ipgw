package cmd

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// The flag parser stops at the first positional argument. Validate all remaining
// arguments before reading credentials, changing configuration or making requests.
func rejectPositionalArguments(ctx *cli.Context) error {
	if ctx.NArg() != 0 {
		return fmt.Errorf("%s 不接受位置参数，请检查选项顺序和拼写", ctx.Command.Name)
	}
	return nil
}

func showCommandGroupHelp(ctx *cli.Context) error {
	if ctx.NArg() != 0 {
		return fmt.Errorf("%s 下存在未知子命令或多余参数，请使用 --help 查看用法", ctx.Command.Name)
	}
	return cli.ShowSubcommandHelp(ctx)
}

func rejectMisplacedOptions(ctx *cli.Context) error {
	// An explicit -- immediately before the positional tail makes even names
	// beginning with '-' unambiguous. Inspect the parent arguments because the
	// flag parser removes that separator from this context's Args.
	lineage := ctx.Lineage()
	if len(lineage) > 1 {
		args := lineage[1].Args().Slice()
		separator := len(args) - ctx.NArg() - 1
		if separator >= 0 && args[separator] == "--" {
			return nil
		}
	}
	for _, arg := range ctx.Args().Slice() {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("%s 的位置参数中存在选项样式的参数，请将选项放在位置参数之前；以 - 开头的位置参数请在整个参数列表前使用 -- 分隔", ctx.Command.Name)
		}
	}
	return nil
}

func validateKickArguments(ctx *cli.Context) error {
	if err := rejectMisplacedOptions(ctx); err != nil {
		return err
	}
	if ctx.NArg() == 0 {
		return fmt.Errorf("请指定要下线的设备 SID")
	}
	for _, sid := range ctx.Args().Slice() {
		if strings.TrimSpace(sid) == "" {
			return fmt.Errorf("设备 SID 不能为空")
		}
	}
	return nil
}
