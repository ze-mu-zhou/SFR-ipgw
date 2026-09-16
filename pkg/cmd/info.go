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

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"github.com/urfave/cli/v2"
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

type infoReport struct {
	SchemaVersion int                     `json:"schema_version"`
	StartedAt     time.Time               `json:"started_at"`
	CompletedAt   time.Time               `json:"completed_at"`
	Status        string                  `json:"status"`
	Error         string                  `json:"error,omitempty"`
	Sections      map[string]querySection `json:"sections"`
}

var InfoCommand = &cli.Command{
	Name: "info", Usage: "query campus billing information", UseShortOptionHandling: true,
	Flags: append(credentialFlags(false),
		&cli.BoolFlag{Name: "all", Aliases: []string{"a"}, Usage: "query all information (first page of each record type)"},
		&cli.BoolFlag{Name: "package", Aliases: []string{"i"}, Usage: "query traffic, balance and package"},
		&cli.BoolFlag{Name: "device", Aliases: []string{"d"}, Usage: "query online devices"},
		&cli.IntFlag{Name: "recharge", Aliases: []string{"r"}, Value: 1, Usage: "recharge records page"},
		&cli.IntFlag{Name: "bill", Aliases: []string{"b"}, Value: 1, Usage: "billing records page"},
		&cli.IntFlag{Name: "log", Aliases: []string{"l"}, Value: 1, Usage: "usage records page"},
		&cli.BoolFlag{Name: "json", Usage: "write structured JSON to stdout"},
		&cli.StringFlag{Name: "snapshot", Usage: "save a successful traffic snapshot to a NEW file"},
		&cli.StringFlag{Name: "period", Usage: "explicitly declare billing period when the server does not provide it"},
		&cli.IntFlag{Name: "traffic-base", Usage: "confirmed unit base for KB/MB/GB: 1000 or 1024"},
	),
	Action:       runInfo,
	OnUsageError: onUsageError,
}

func runInfo(ctx *cli.Context) error {
	report := &infoReport{SchemaVersion: 1, StartedAt: time.Now().UTC(), Status: "ok", Sections: map[string]querySection{}}
	var queryErr error
	var account *model.Account
	err := validateInfoOptions(ctx)
	if err == nil {
		account, err = getAccountByContext(ctx)
	}
	if err != nil {
		report.Sections["authentication"] = querySection{Status: "error", Error: err.Error()}
		queryErr = err
	} else {
		h := handler.NewDashboardHandler()
		if err = h.Login(account); err != nil {
			report.Sections["authentication"] = querySection{Status: "error", Error: err.Error()}
			queryErr = fmt.Errorf("登录失败：%w", err)
		} else {
			queryErr = collectInfo(ctx, h, report)
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
		enc := json.NewEncoder(ctx.App.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
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
		v, e := reader.GetPackage()
		record("package", v, e)
	}
	if all || ctx.Bool("device") {
		v, e := reader.GetDevice()
		record("devices", v, e)
	}
	if all || ctx.IsSet("log") {
		v, e := reader.GetUsageRecords(ctx.Int("log"))
		record("usage", v, e)
	}
	if all || ctx.IsSet("bill") {
		v, e := reader.GetBill(ctx.Int("bill"))
		record("bills", v, e)
	}
	if all || ctx.IsSet("recharge") {
		v, e := reader.GetRecharge(ctx.Int("recharge"))
		record("recharges", v, e)
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func printInfoReport(w io.Writer, r *infoReport) {
	titles := map[string]string{"authentication": "登录", "basic": "基本信息", "package": "套餐信息", "devices": "在线设备", "usage": "使用历史", "bills": "扣费记录", "recharges": "充值记录", "snapshot": "快照"}
	for _, name := range []string{"authentication", "basic", "package", "devices", "usage", "bills", "recharges", "snapshot"} {
		section, ok := r.Sections[name]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "# %s\n", titles[name])
		if section.Status != "ok" {
			fmt.Fprintf(w, "  失败：%s\n\n", section.Error)
			continue
		}
		switch v := section.Data.(type) {
		case *handler.Basic:
			fmt.Fprintf(w, "  姓名：%s\n  学号：%s\n", v.Name, v.ID)
		case *handler.Package:
			fmt.Fprintf(w, "  已用：%s\n  时长：%s\n  消费：%s 元\n  余额：%s 元\n", v.UsedTraffic, v.UsedDuration, v.PackageCost, v.Balance)
			if v.BillingPeriod != "" {
				fmt.Fprintf(w, "  计费周期：%s\n", v.BillingPeriod)
			}
		case []handler.Device:
			if len(v) == 0 {
				fmt.Fprintln(w, "  无在线设备")
			}
			for _, x := range v {
				fmt.Fprintf(w, "  #%d  %s  %s  SID=%s  %s\n", x.ID, x.IP, x.StartTime, x.SID, x.Stage)
			}
		case []handler.UsageRecord:
			if len(v) == 0 {
				fmt.Fprintln(w, "  无记录")
			}
			for _, x := range v {
				fmt.Fprintf(w, "  %s — %s  %s  %s  %s\n", x.StartTime, x.EndTime, x.IP, x.Traffic, x.UsedDuration)
			}
		case []handler.BillRecord:
			if len(v) == 0 {
				fmt.Fprintln(w, "  无记录")
			}
			for _, x := range v {
				fmt.Fprintf(w, "  #%s  %s  %.2f 元  %s\n", x.ID, x.Date, x.Cost, x.Traffic)
			}
		case []handler.RechargeRecord:
			if len(v) == 0 {
				fmt.Fprintln(w, "  无记录")
			}
			for _, x := range v {
				fmt.Fprintf(w, "  #%s  %s  %s 元\n", x.ID, x.Time, x.Cost)
			}
		case map[string]string:
			fmt.Fprintf(w, "  已保存：%s\n", v["path"])
		}
		fmt.Fprintln(w)
	}
}

func snapshotFromReport(r *infoReport, period string, base int) (*model.Snapshot, error) {
	for _, s := range r.Sections {
		if s.Status != "ok" {
			return nil, errors.New("查询包含失败项，不能保存有效快照")
		}
	}
	basic, ok := r.Sections["basic"].Data.(*handler.Basic)
	if !ok || basic == nil || basic.ID == "" {
		return nil, errors.New("快照缺少已确认账号")
	}
	pkg, ok := r.Sections["package"].Data.(*handler.Package)
	if !ok || pkg == nil {
		return nil, errors.New("快照缺少用量数据")
	}
	m, err := model.ParseTraffic(pkg.UsedTraffic, base)
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
	return &model.Snapshot{SchemaVersion: 1, AccountID: basic.ID, CapturedAt: r.CompletedAt, QueryStatus: "ok", Source: "ipgw-dashboard", BillingPeriod: period, PeriodSource: source, Traffic: m}, nil
}

func writeNewJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	success = true
	return nil
}
