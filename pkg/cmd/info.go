package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

type dashboardReader interface {
	GetBasic() (*handler.Basic, error)
	GetPackage() (*handler.Package, error)
	GetDevice() ([]handler.Device, error)
	GetRecharge(int) ([]handler.RechargeRecord, error)
	GetUsageRecords(int) ([]handler.UsageRecord, error)
	GetBill(int) ([]handler.BillRecord, error)
}

type querySection struct {
	Status string `json:"status"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
}

// infoReport 的 schema_version 在 JSON 字段含义或类型变化时递增。
// 版本 2：bills[].cost 由数字改为十进制字符串，与其他金额字段一致。
type infoReport struct {
	SchemaVersion int                     `json:"schema_version"`
	StartedAt     time.Time               `json:"started_at"`
	CompletedAt   time.Time               `json:"completed_at"`
	Status        string                  `json:"status"`
	Error         string                  `json:"error,omitempty"`
	Sections      map[string]querySection `json:"sections"`
}

var InfoCommand = &cli.Command{
	Name:                   "info",
	Usage:                  "查询校园网计费信息",
	UseShortOptionHandling: true,
	Flags: append(credentialFlags(),
		&cli.BoolFlag{Name: "all", Aliases: []string{"a"}, Usage: "查询全部信息（各类记录只查第一页）"},
		&cli.BoolFlag{Name: "package", Aliases: []string{"i"}, Usage: "查询流量、余额和套餐"},
		&cli.BoolFlag{Name: "device", Aliases: []string{"d"}, Usage: "查询在线设备"},
		&cli.IntFlag{Name: "recharge", Aliases: []string{"r"}, Value: 1, Usage: "充值记录`页码`"},
		&cli.IntFlag{Name: "bill", Aliases: []string{"b"}, Value: 1, Usage: "扣费记录`页码`"},
		&cli.IntFlag{Name: "log", Aliases: []string{"l"}, Value: 1, Usage: "使用记录`页码`"},
		&cli.BoolFlag{Name: "json", Usage: "以 JSON 格式输出到标准输出"},
		&cli.StringFlag{Name: "snapshot", Usage: "把成功查询的流量快照保存到新`文件`（文件不能已存在）"},
		&cli.StringFlag{Name: "period", Usage: "服务器未提供计费周期时，手动声明计费`周期`"},
		&cli.IntFlag{Name: "traffic-base", Usage: "已确认的 KB/MB/GB 单位`基数`：1000 或 1024", DefaultText: "未指定"},
	),
	Action:       runInfo,
	OnUsageError: onUsageError,
}

func runInfo(ctx *cli.Context) error {
	report := &infoReport{SchemaVersion: 2, StartedAt: time.Now().UTC(), Status: "ok", Sections: map[string]querySection{}}
	var queryErr error
	var account *model.Account
	err := validateInfoOptions(ctx)
	if err == nil {
		account, err = accountFromContext(ctx)
	}
	if err != nil {
		report.Sections["authentication"] = querySection{Status: "error", Error: err.Error()}
		queryErr = err
	} else {
		dashboard := handler.NewDashboardHandler()
		if err = dashboard.Login(account); err != nil {
			report.Sections["authentication"] = querySection{Status: "error", Error: err.Error()}
			queryErr = fmt.Errorf("登录失败：%w", err)
		} else {
			queryErr = collectInfo(ctx, dashboard, report)
		}
	}
	report.CompletedAt = time.Now().UTC()
	if queryErr == nil && ctx.String("snapshot") != "" {
		var snapshot *model.Snapshot
		snapshot, err = snapshotFromReport(report, ctx.String("period"), ctx.Int("traffic-base"))
		if err == nil {
			err = writeNewJSON(ctx.String("snapshot"), snapshot)
		}
		if err != nil {
			report.Sections["snapshot"] = querySection{Status: "error", Error: err.Error()}
			queryErr = fmt.Errorf("保存快照失败：%w", err)
		} else {
			report.Sections["snapshot"] = querySection{Status: "ok", Data: map[string]string{"path": ctx.String("snapshot")}}
		}
	}
	if queryErr != nil {
		report.Status = "error"
		report.Error = queryErr.Error()
	}
	if ctx.Bool("json") {
		encoder := json.NewEncoder(ctx.App.Writer)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		printInfoReport(ctx.App.Writer, report)
	}
	return queryErr
}

func validateInfoOptions(ctx *cli.Context) error {
	if ctx.NArg() != 0 {
		return errors.New("info 不接受位置参数，请检查选项顺序和拼写")
	}
	if ctx.IsSet("snapshot") && strings.TrimSpace(ctx.String("snapshot")) == "" {
		return errors.New("--snapshot 文件名不能为空")
	}
	if ctx.IsSet("traffic-base") && ctx.Int("traffic-base") == 0 {
		return errors.New("--traffic-base 必须明确指定 1000 或 1024")
	}
	for _, name := range []string{"log", "bill", "recharge"} {
		if ctx.Int(name) < 1 {
			return fmt.Errorf("%s 页码必须大于零", name)
		}
	}
	if base := ctx.Int("traffic-base"); base != 0 && base != 1000 && base != 1024 {
		return errors.New("--traffic-base 只能是 1000 或 1024")
	}
	if (ctx.IsSet("period") || ctx.IsSet("traffic-base")) && ctx.String("snapshot") == "" {
		return errors.New("--period 和 --traffic-base 需要与 --snapshot 一起使用")
	}
	if path := ctx.String("snapshot"); path != "" {
		parent, err := os.Stat(filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("快照目录不可用：%w", err)
		}
		if !parent.IsDir() {
			return errors.New("快照父路径不是目录")
		}
		if _, err := os.Lstat(path); err == nil {
			return errors.New("快照文件已存在，请使用新文件名")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func collectInfo(ctx *cli.Context, reader dashboardReader, report *infoReport) error {
	for _, name := range []string{"log", "bill", "recharge"} {
		if ctx.Int(name) < 1 {
			return fmt.Errorf("%s 页码必须大于零", name)
		}
	}
	var failures []string
	record := func(name string, data any, err error) {
		if err != nil {
			report.Sections[name] = querySection{Status: "error", Error: err.Error()}
			failures = append(failures, name+": "+err.Error())
		} else {
			report.Sections[name] = querySection{Status: "ok", Data: data}
		}
	}
	basic, err := reader.GetBasic()
	record("basic", basic, err)
	all := ctx.Bool("all")
	if all || ctx.Bool("package") || ctx.String("snapshot") != "" {
		pkg, err := reader.GetPackage()
		record("package", pkg, err)
	}
	if all || ctx.Bool("device") {
		devices, err := reader.GetDevice()
		record("devices", devices, err)
	}
	if all || ctx.IsSet("log") {
		usages, err := reader.GetUsageRecords(ctx.Int("log"))
		record("usage", usages, err)
	}
	if all || ctx.IsSet("bill") {
		bills, err := reader.GetBill(ctx.Int("bill"))
		record("bills", bills, err)
	}
	if all || ctx.IsSet("recharge") {
		recharges, err := reader.GetRecharge(ctx.Int("recharge"))
		record("recharges", recharges, err)
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func printInfoReport(w io.Writer, report *infoReport) {
	titles := map[string]string{"authentication": "登录", "basic": "基本信息", "package": "套餐信息", "devices": "在线设备", "usage": "使用历史", "bills": "扣费记录", "recharges": "充值记录", "snapshot": "快照"}
	for _, name := range []string{"authentication", "basic", "package", "devices", "usage", "bills", "recharges", "snapshot"} {
		section, ok := report.Sections[name]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "# %s\n", titles[name])
		if section.Status != "ok" {
			fmt.Fprintf(w, "  失败：%s\n\n", section.Error)
			continue
		}
		switch data := section.Data.(type) {
		case *handler.Basic:
			fmt.Fprintf(w, "  姓名：%s\n  学号：%s\n", data.Name, data.ID)
		case *handler.Package:
			fmt.Fprintf(w, "  已用：%s\n  时长：%s\n  消费：%s 元\n  余额：%s 元\n", data.UsedTraffic, data.UsedDuration, data.PackageCost, data.Balance)
			if data.BillingPeriod != "" {
				fmt.Fprintf(w, "  计费周期：%s\n", data.BillingPeriod)
			}
		case []handler.Device:
			if len(data) == 0 {
				fmt.Fprintln(w, "  无在线设备")
			}
			for _, device := range data {
				fmt.Fprintf(w, "  #%d  %s  %s  SID=%s  %s\n", device.ID, device.IP, device.StartTime, device.SID, device.Stage)
			}
		case []handler.UsageRecord:
			if len(data) == 0 {
				fmt.Fprintln(w, "  无记录")
			}
			for _, usage := range data {
				fmt.Fprintf(w, "  %s — %s  %s  %s  %s\n", usage.StartTime, usage.EndTime, usage.IP, usage.Traffic, usage.UsedDuration)
			}
		case []handler.BillRecord:
			if len(data) == 0 {
				fmt.Fprintln(w, "  无记录")
			}
			for _, bill := range data {
				fmt.Fprintf(w, "  #%s  %s  %s 元  %s\n", bill.ID, bill.Date, bill.Cost, bill.Traffic)
			}
		case []handler.RechargeRecord:
			if len(data) == 0 {
				fmt.Fprintln(w, "  无记录")
			}
			for _, recharge := range data {
				fmt.Fprintf(w, "  #%s  %s  %s 元\n", recharge.ID, recharge.Time, recharge.Cost)
			}
		case map[string]string:
			fmt.Fprintf(w, "  已保存：%s\n", data["path"])
		}
		fmt.Fprintln(w)
	}
}

func snapshotFromReport(report *infoReport, period string, base int) (*model.Snapshot, error) {
	for _, section := range report.Sections {
		if section.Status != "ok" {
			return nil, errors.New("查询包含失败项，不能保存有效快照")
		}
	}
	basic, ok := report.Sections["basic"].Data.(*handler.Basic)
	if !ok || basic == nil || basic.ID == "" {
		return nil, errors.New("快照缺少已确认账号")
	}
	pkg, ok := report.Sections["package"].Data.(*handler.Package)
	if !ok || pkg == nil {
		return nil, errors.New("快照缺少用量数据")
	}
	traffic, err := model.ParseTraffic(pkg.UsedTraffic, base)
	if err != nil {
		return nil, err
	}
	period = strings.TrimSpace(period)
	source := "unknown"
	if pkg.BillingPeriod != "" {
		if period != "" && period != pkg.BillingPeriod {
			return nil, errors.New("声明周期与服务器周期不一致")
		}
		period, source = pkg.BillingPeriod, "server"
	} else if period != "" {
		source = "user"
	}
	return &model.Snapshot{SchemaVersion: 1, AccountID: basic.ID, CapturedAt: report.CompletedAt, QueryStatus: "ok", Source: "ipgw-dashboard", BillingPeriod: period, PeriodSource: source, Traffic: traffic}, nil
}

func writeNewJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		file.Close()
		if !success {
			os.Remove(path)
		}
	}()
	if _, err = file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	success = true
	return nil
}
