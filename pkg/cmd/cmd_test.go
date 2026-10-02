package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestLocalizeError(t *testing.T) {
	for _, tc := range []struct {
		input, want string
	}{
		{"flag provided but not defined: -x", "未知选项：-x"},
		{"flag needs an argument: -u", "选项 -u 缺少参数值"},
		{`invalid value "abc" for flag -r: parse error`, `选项 -r 的值 "abc" 无效`},
		{`Required flag "username" not set`, "缺少必需选项：username"},
		{`Required flags "username, config" not set`, "缺少必需选项：username, config"},
		{"No help topic for 'typo'", "没有 'typo' 的帮助主题"},
		{"网关登录失败", "网关登录失败"},
	} {
		if got := localizeError(errors.New(tc.input)).Error(); got != tc.want {
			t.Errorf("localizeError(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	if localizeError(nil) != nil {
		t.Error("nil error was replaced")
	}
}

func TestHelpIsChinese(t *testing.T) {
	var output bytes.Buffer
	writer := App.Writer
	App.Writer = &output
	t.Cleanup(func() { App.Writer = writer })
	for _, args := range [][]string{{"ipgw", "--help"}, {"ipgw", "info", "--help"}, {"ipgw", "config", "account", "--help"}} {
		output.Reset()
		if err := Run(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		text := output.String()
		for _, english := range []string{"USAGE", "OPTIONS", "COMMANDS", "show help", "Shows a list", "(default:"} {
			if strings.Contains(text, english) {
				t.Errorf("%v help contains %q:\n%s", args, english, text)
			}
		}
		if !strings.Contains(text, "用法：") {
			t.Errorf("%v help lacks Chinese usage header:\n%s", args, text)
		}
	}
}

// Every command must localize usage errors and show its help, like the others.
func TestAllCommandsHandleUsageErrors(t *testing.T) {
	var check func(prefix string, commands []*cli.Command)
	check = func(prefix string, commands []*cli.Command) {
		for _, command := range commands {
			if command.Name == "help" { // urfave/cli 内置命令
				continue
			}
			name := strings.TrimSpace(prefix + " " + command.Name)
			if command.OnUsageError == nil {
				t.Errorf("%s has no OnUsageError", name)
			}
			check(name, command.Subcommands)
		}
	}
	check("", App.Commands)
}

func TestFormatError(t *testing.T) {
	cause := errors.New("当前不在校园网内")
	wrapped := fmt.Errorf("修改账号失败：%w", fmt.Errorf("设置密码失败：%w", cause))
	if !errors.Is(wrapped, cause) {
		t.Fatal("wrapping lost the original error")
	}
	for _, tc := range []struct {
		err  error
		want string
	}{
		{cause, "当前不在校园网内"},
		{fmt.Errorf("登录失败：%w", cause), "登录失败：\n\t当前不在校园网内"},
		{wrapped, "修改账号失败：\n\t设置密码失败：\n\t\t当前不在校园网内"},
		// 无前缀的透传包装和多重 %w 不拆分，保持原文。
		{fmt.Errorf("%w", cause), "当前不在校园网内"},
		{fmt.Errorf("安装失败：%w；回滚失败：%w", cause, cause), "安装失败：当前不在校园网内；回滚失败：当前不在校园网内"},
	} {
		if got := FormatError(tc.err); got != tc.want {
			t.Errorf("FormatError(%q) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
