package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
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
