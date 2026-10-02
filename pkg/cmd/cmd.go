package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

var App = &cli.App{
	Name:      "ipgw",
	HelpName:  "ipgw",
	Usage:     "东北大学校园网网关命令行工具",
	Copyright: "项目主页：\thttps://github.com/ze-mu-zhou/SFR-ipgw\n问题反馈：\thttps://github.com/ze-mu-zhou/SFR-ipgw/issues/new",
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
			Usage:   "从指定`文件`加载配置（默认 ~/.ipgw）",
		},
	},
	OnUsageError: onUsageError,
	Writer:       console.Stdout,
	ErrWriter:    console.Stderr,
}

// Run 运行命令行程序，并把命令行解析库返回的英文错误转换为中文。
func Run(args []string) error {
	localizeHelpCommand()
	return localizeError(App.Run(args))
}

// urfave/cli 在 Setup 时追加内置 help 命令，各级命令共用同一个实例，
// 修改一次即可覆盖所有层级。Setup 可重复调用。
func localizeHelpCommand() {
	App.Setup()
	if help := App.Command("help"); help != nil {
		help.Usage = "显示命令列表，或显示某个命令的帮助"
		help.ArgsUsage = "[命令]"
	}
}

func loginUseDefaultAccount(ctx *cli.Context) error {
	account, err := accountFromContext(ctx)
	if err != nil {
		return err
	}
	console.Infof("使用账号 '%s'\n", account.Username)

	if err = login(handler.NewIPGWHandler(), account); err != nil {
		return fmt.Errorf("登录失败：%w", err)
	}
	return nil
}

func accountFromContext(ctx *cli.Context) (account *model.Account, err error) {
	username := ctx.String("username")
	if username != "" && ctx.Bool("ask-password") {
		password, err := promptPassword(ctx)
		if err != nil {
			return nil, err
		}
		return &model.Account{Username: username, Password: password}, nil
	}
	store, err := loadStore(ctx)
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

func loadStore(ctx *cli.Context) (*handler.StoreHandler, error) {
	store, err := handler.NewStoreHandler(ctx.String("config"))
	if err != nil {
		return nil, err
	}
	if err = store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

func onUsageError(ctx *cli.Context, err error, isSubcommand bool) error {
	_, _ = fmt.Fprintf(ctx.App.Writer, "%s\n\n", localizeError(err))
	if isSubcommand {
		cli.ShowSubcommandHelpAndExit(ctx, 1)
	} else {
		cli.ShowAppHelpAndExit(ctx, 1)
	}
	return nil
}

// 标准库 flag 包和 urfave/cli 的错误文案固定为英文，按格式逐条转换。
var errorTranslations = []struct {
	pattern *regexp.Regexp
	message string
}{
	{regexp.MustCompile(`^flag provided but not defined: (-+\S+)$`), "未知选项：$1"},
	{regexp.MustCompile(`^flag needs an argument: (-+\S+)$`), "选项 $1 缺少参数值"},
	{regexp.MustCompile(`^invalid value (".*") for flag (-+\S+): .*$`), "选项 $2 的值 $1 无效"},
	{regexp.MustCompile(`^invalid boolean value (".*") for (-+\S+): .*$`), "选项 $2 的值 $1 不是有效的布尔值"},
	{regexp.MustCompile(`^invalid boolean flag (\S+): .*$`), "选项 $1 不接受该值"},
	{regexp.MustCompile(`^bad flag syntax: (.*)$`), "选项格式错误：$1"},
	{regexp.MustCompile(`^Required flags? "(.*)" not set$`), "缺少必需选项：$1"},
	{regexp.MustCompile(`^No help topic for '(.*)'$`), "没有 '$1' 的帮助主题"},
}

func localizeError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, translation := range errorTranslations {
		if translation.pattern.MatchString(message) {
			return errors.New(translation.pattern.ReplaceAllString(message, translation.message))
		}
	}
	return err
}

// FormatError 把 fmt.Errorf("前缀：%w", err) 形成的错误链按层拆成多行，
// 内层逐级缩进。错误值本身保持单行，排版只在最终输出时进行。
func FormatError(err error) string {
	var text strings.Builder
	for depth := 0; err != nil; depth++ {
		if depth > 0 {
			text.WriteString("\n" + strings.Repeat("\t", depth))
		}
		message := err.Error()
		inner := errors.Unwrap(err)
		if inner != nil {
			if prefix, ok := strings.CutSuffix(message, inner.Error()); ok && strings.TrimSpace(prefix) != "" {
				text.WriteString(strings.TrimSpace(prefix))
				err = inner
				continue
			}
		}
		text.WriteString(message)
		break
	}
	return text.String()
}

func init() {
	cli.HelpFlag = &cli.BoolFlag{
		Name:               "help",
		Aliases:            []string{"h"},
		Usage:              "显示帮助",
		DisableDefaultText: true,
	}
	defaultFlagStringer := cli.FlagStringer
	cli.FlagStringer = func(flag cli.Flag) string {
		text := strings.TrimSuffix(defaultFlagStringer(flag), " (default: false)")
		return strings.Replace(text, " (default: ", " (默认：", 1)
	}
	cli.AppHelpTemplate = `{{.Usage}}

用法：
   {{.HelpName}} {{if .VisibleFlags}}[全局选项]{{end}}{{if .Commands}} 命令 [命令选项]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[参数...]{{end}}
{{if .Commands}}
命令：
{{range .Commands}}{{if not .HideHelp}}   {{join .Names ", "}}{{ "\t"}}{{.Usage}}{{ "\n" }}{{end}}{{end}}{{end}}
选项：
   {{range .VisibleFlags}}{{.}}
   {{end}}
{{.Copyright}}
`
	cli.CommandHelpTemplate = `{{.Usage}}

用法：
   {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}}{{if .VisibleFlags}} [命令选项]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[参数...]{{end}}{{end}}{{if .Category}}

分类：
   {{.Category}}{{end}}{{if .Description}}

说明：
   {{.Description | nindent 3 | trim}}{{end}}{{if .VisibleFlags}}

选项：
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
`
	cli.SubcommandHelpTemplate = `{{.Usage}}

用法：
   {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} 命令{{if .VisibleFlags}} [命令选项]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[参数...]{{end}}{{end}}{{if .Description}}

说明：
   {{.Description | nindent 3 | trim}}{{end}}

命令：{{range .VisibleCategories}}{{if .Name}}
   {{.Name}}:{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{else}}{{range .VisibleCommands}}
   {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{if .VisibleFlags}}

选项：
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
`
}
