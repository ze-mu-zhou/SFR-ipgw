package handler

import (
	"encoding/json"
	"fmt"
	"testing"
)

func packageTable(balance string) string {
	return `<table><tr><th data-col-seq="3">已用流量</th><th data-col-seq="4">已用时长</th><th data-col-seq="6">套餐费用</th><th data-col-seq="7">余额</th></tr><tr><td data-col-seq="3">1 GB</td><td data-col-seq="4">1小时</td><td data-col-seq="6">20</td><td data-col-seq="7">` + balance + `</td></tr></table>`
}

func TestPackageSelectionDoesNotDependOnTableOrder(t *testing.T) {
	for _, stage := range []string{"在线", "1"} {
		device := `<table><tr><th data-col-seq="3">上线时间</th><th data-col-seq="4">使用时长</th><th data-col-seq="6">MAC地址</th><th data-col-seq="7">状态</th></tr><tr data-key="session"><td data-col-seq="3">2026-09-01 10:00:00</td><td data-col-seq="4">1小时</td><td data-col-seq="6">00:11:22:33:44:55</td><td data-col-seq="7">` + stage + `</td></tr></table>`
		for _, body := range []string{device + packageTable("10"), packageTable("10") + device} {
			p, err := dashboardFixture(body).GetPackage()
			if err != nil || p.UsedTraffic != "1 GB" || p.Balance != "10" || p.Overdue {
				t.Fatalf("wrong package: %+v error=%v", p, err)
			}
		}
		if _, err := dashboardFixture(device).GetPackage(); err == nil {
			t.Fatal("device-only page was accepted as a package")
		}
	}
}

func TestPackageHeaderMappingAndColumnFallback(t *testing.T) {
	for _, body := range []string{
		// Reordered columns must follow their semantic headers.
		`<table><tr><th>账户余额</th><th>消费</th><th>使用时长</th><th>使用流量</th></tr><tr><td>-10</td><td>20</td><td>1小时</td><td>1 GB</td></tr></table>`,
		// After identifying the table, legacy column IDs still work for other fields.
		`<table><tr><th data-col-seq="3">使用流量</th><th data-col-seq="7">账户余额</th></tr><tr><td data-col-seq="3">1 GB</td><td data-col-seq="4">1小时</td><td data-col-seq="6">20</td><td data-col-seq="7">-10</td></tr></table>`,
	} {
		p, err := dashboardFixture(body).GetPackage()
		if err != nil || p.UsedTraffic != "1 GB" || p.PackageCost != "20" || p.UsedDuration != "1小时" || !p.Overdue {
			t.Fatalf("wrong package: %+v error=%v", p, err)
		}
	}
	// Column numbers alone do not establish the meaning of a table.
	if _, err := dashboardFixture(`<table><tr><td data-col-seq="3">1 GB</td><td data-col-seq="4">1小时</td><td data-col-seq="6">20</td><td data-col-seq="7">10</td></tr></table>`).GetPackage(); err == nil {
		t.Fatal("unidentified table was accepted as a package")
	}
}

func TestPackageRejectsNonFiniteBalance(t *testing.T) {
	for _, balance := range []string{"NaN", "+Inf", "-Inf", "Infinity", "1e999", "invalid"} {
		t.Run(balance, func(t *testing.T) {
			if _, err := dashboardFixture(packageTable(balance)).GetPackage(); err == nil {
				t.Fatal("invalid balance was accepted")
			}
		})
	}
}

func TestBillRejectsNonFiniteAmounts(t *testing.T) {
	for _, tc := range []struct {
		fixed, variable string
		bad             bool
	}{
		{"NaN", "0", true}, {"0", "NaN", true}, {"+Inf", "0", true},
		{"0", "-Inf", true}, {"Infinity", "0", true}, {"1e999", "0", true},
		{"1e308", "1e308", true}, {"-1e308", "-1e308", true},
		{"5.25", "0.75", false}, {"10", "-2", false},
	} {
		t.Run(tc.fixed+"/"+tc.variable, func(t *testing.T) {
			body := fmt.Sprintf(`<title>结算清单</title><table><tr><td data-col-seq="0">1</td><td data-col-seq="2">%s</td><td data-col-seq="3">%s</td><td data-col-seq="7">1 GB</td><td data-col-seq="10">1小时</td><td data-col-seq="12">2026-09-01</td></tr></table>`, tc.fixed, tc.variable)
			rows, err := dashboardFixture(body).GetBill(1)
			if (err != nil) != tc.bad {
				t.Fatalf("rows=%+v error=%v", rows, err)
			}
			if err == nil {
				if _, err := json.Marshal(rows); err != nil {
					t.Fatalf("successful query cannot be encoded as JSON: %v", err)
				}
			}
		})
	}
}

func TestBasicAndDevices(t *testing.T) {
	body := `<div><label>用户名</label><span>alice</span></div><div><label>姓名</label><span>张三</span></div>` + packageTable("10") +
		`<table><tr><th data-col-seq="1">IP地址</th><th data-col-seq="3">上线时间</th><th data-col-seq="7">状态</th></tr><tr data-key="session-1"><td data-col-seq="1">192.0.2.1</td><td data-col-seq="3">2026-09-01 10:00:00</td><td data-col-seq="7">在线</td></tr></table>`
	d := dashboardFixture(body)
	basic, err := d.GetBasic()
	if err != nil || basic.ID != "alice" || basic.Name != "张三" {
		t.Fatalf("basic=%+v error=%v", basic, err)
	}
	devices, err := d.GetDevice()
	if err != nil || len(devices) != 1 || devices[0].SID != "session-1" || devices[0].IP != "192.0.2.1" || devices[0].Stage != "在线" {
		t.Fatalf("devices=%+v error=%v", devices, err)
	}
}

func TestBasicAndDevicesRejectMissingFields(t *testing.T) {
	if _, err := dashboardFixture(`<div><label>用户名</label>alice</div>`).GetBasic(); err == nil {
		t.Fatal("missing name was accepted")
	}
	for _, body := range []string{
		`<table><tr><th>IP地址</th></tr></table>`,
		`<table><tr><th data-col-seq="1">IP地址</th></tr><tr><td data-col-seq="1">192.0.2.1</td><td data-col-seq="3">2026-09-01</td><td data-col-seq="7">在线</td></tr></table>`,
		packageTable("10"),
	} {
		if _, err := dashboardFixture(body).GetDevice(); err == nil {
			t.Fatal("missing devices or SID was accepted")
		}
	}
	rows, err := dashboardFixture(`<table><tr><th>IP地址</th></tr><tr><td colspan="10"><div class="empty">没有找到数据。</div></td></tr></table>`).GetDevice()
	if err != nil || len(rows) != 0 {
		t.Fatalf("explicit empty grid: %+v %v", rows, err)
	}
}
