package handler

import (
	"errors"
	"fmt"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"golang.org/x/net/html"
	"net/http"
	"strconv"
	"strings"
)

type DashboardHandler struct {
	client                      *http.Client
	cachedDashboardIndexContent string
}

func NewDashboardHandler() *DashboardHandler { return &DashboardHandler{client: newSession()} }
func (d *DashboardHandler) Login(a *model.Account) error {
	p, e := a.GetPassword()
	if e != nil {
		return e
	}
	if e = loginCAS(d.client, casLoginURL, a.Username, p); e != nil {
		return e
	}
	_, e = dashboardPage(d.client, "/sso/neusoft/index")
	d.cachedDashboardIndexContent = ""
	return e
}
func (d *DashboardHandler) getCachedDashboardIndexBody() (string, error) {
	if d.cachedDashboardIndexContent == "" {
		b, e := dashboardPage(d.client, "/home")
		if e != nil {
			return "", e
		}
		d.cachedDashboardIndexContent = b
	}
	return d.cachedDashboardIndexContent, nil
}

type Basic struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Package struct {
	BillingPeriod string `json:"billing_period,omitempty"`
	PackageCost   string `json:"package_cost"`
	UsedTraffic   string `json:"used_traffic"`
	UsedDuration  string `json:"used_duration"`
	Balance       string `json:"balance"`
	Overdue       bool   `json:"overdue"`
}
type Device struct {
	ID        int    `json:"id"`
	IP        string `json:"ip"`
	StartTime string `json:"start_time"`
	Stage     string `json:"stage"`
	SID       string `json:"sid"`
}
type BillRecord struct {
	ID           string  `json:"id"`
	Cost         float64 `json:"cost"`
	Traffic      string  `json:"traffic"`
	UsedDuration string  `json:"used_duration"`
	Date         string  `json:"date"`
}
type UsageRecord struct {
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	IP           string `json:"ip"`
	Traffic      string `json:"traffic"`
	UsedDuration string `json:"used_duration"`
}
type RechargeRecord struct {
	ID   string `json:"id"`
	Cost string `json:"cost"`
	Time string `json:"time"`
}

func (d *DashboardHandler) indexDOM() (*html.Node, error) {
	b, e := d.getCachedDashboardIndexBody()
	if e != nil {
		return nil, e
	}
	return pageDOM(b)
}
func (d *DashboardHandler) GetBasic() (*Basic, error) {
	doc, e := d.indexDOM()
	if e != nil {
		return nil, e
	}
	vals := map[string]string{}
	for _, n := range nodes(doc, "label") {
		if n.Parent != nil {
			label := nodeText(n)
			vals[label] = strings.TrimSpace(strings.TrimPrefix(nodeText(n.Parent), label))
		}
	}
	if vals["用户名"] == "" || vals["姓名"] == "" {
		return nil, pageFormatError()
	}
	return &Basic{vals["用户名"], vals["姓名"]}, nil
}

// Read cell text from the DOM. Header names take precedence; known column IDs
// support the existing Yii grids when headers are abbreviated or absent.
type gridRow struct {
	cells map[string]string
	sid   string
}

func tableRows(t *html.Node) []gridRow {
	headers := map[string]string{}
	for i, h := range nodes(t, "th") {
		key := attr(h, "data-col-seq")
		if key == "" {
			key = strconv.Itoa(i)
		}
		headers[key] = nodeText(h)
	}
	var out []gridRow
	for _, r := range nodes(t, "tr") {
		if !isDataRow(r, t) {
			continue
		}
		row := gridRow{cells: map[string]string{}, sid: attr(r, "data-key")}
		for i, c := range nodes(r, "td") {
			key := attr(c, "data-col-seq")
			if key == "" && attr(c, "colspan") == "" {
				key = strconv.Itoa(i)
			}
			if key != "" {
				row.cells[key] = nodeText(c)
				if headers[key] != "" {
					row.cells[headers[key]] = nodeText(c)
				}
			}
		}
		if len(row.cells) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func isDataRow(r, table *html.Node) bool {
	// Yii/Kartik puts page summaries in tbody as well as tfoot. They have
	// ordinary td cells, but must not be parsed as incomplete records.
	for _, class := range strings.Fields(attr(r, "class")) {
		switch class {
		case "kv-page-summary", "filters":
			return false
		}
	}
	for p := r.Parent; p != nil && p != table; p = p.Parent {
		if p.Type == html.ElementNode {
			switch p.Data {
			case "thead", "tfoot", "table":
				return false
			}
		}
	}
	return true
}

func field(row gridRow, id string, names ...string) string {
	for _, n := range names {
		if v := row.cells[n]; v != "" {
			return v
		}
	}
	return row.cells[id]
}
func required(values ...string) bool {
	for _, s := range values {
		if strings.TrimSpace(s) == "" {
			return false
		}
	}
	return true
}
func (d *DashboardHandler) GetPackage() (*Package, error) {
	doc, e := d.indexDOM()
	if e != nil {
		return nil, e
	}
	for _, t := range nodes(doc, "table") {
		for _, r := range tableRows(t) {
			p := &Package{UsedTraffic: field(r, "3", "已用流量", "使用流量"), UsedDuration: field(r, "4", "已用时长", "使用时长"), PackageCost: field(r, "6", "套餐费用", "消费"), Balance: field(r, "7", "余额", "账户余额")}
			if !required(p.UsedTraffic, p.UsedDuration, p.PackageCost, p.Balance) {
				continue
			}
			v, e := strconv.ParseFloat(p.Balance, 64)
			if e != nil {
				return nil, errors.New("账单页面中的余额无效")
			}
			p.Overdue = v < 0
			p.BillingPeriod = field(r, "", "计费周期", "账期")
			return p, nil
		}
	}
	return nil, pageFormatError()
}
func explicitEmpty(t *html.Node) bool {
	for _, n := range nodes(t, "td") {
		for _, c := range strings.Fields(attr(n, "class")) {
			if c == "empty" {
				return true
			}
		}
		for _, d := range nodes(n, "div") {
			for _, c := range strings.Fields(attr(d, "class")) {
				if c == "empty" {
					return true
				}
			}
		}
	}
	return false
}
func (d *DashboardHandler) GetDevice() ([]Device, error) {
	doc, e := d.indexDOM()
	if e != nil {
		return nil, e
	}
	result := []Device{}
	for _, t := range nodes(doc, "table") {
		rows := tableRows(t)
		isDevices := false
		for _, h := range nodes(t, "th") {
			s := nodeText(h)
			if strings.Contains(s, "IP") || strings.Contains(s, "上线时间") {
				isDevices = true
			}
		}
		for _, r := range rows {
			if r.cells["9"] != "" && r.sid != "" {
				isDevices = true
			}
		}
		if !isDevices {
			continue
		}
		if explicitEmpty(t) {
			return result, nil
		}
		for _, r := range rows {
			ip, start, stage := field(r, "1", "IP地址", "IP"), field(r, "3", "上线时间"), field(r, "7", "状态")
			if !required(ip, start, stage, r.sid) {
				return nil, pageFormatError()
			}
			result = append(result, Device{len(result), ip, start, stage, r.sid})
		}
		if len(result) == 0 {
			return nil, pageFormatError()
		}
		return result, nil
	}
	return nil, pageFormatError()
}
func (d *DashboardHandler) records(path, title string, page int) ([]gridRow, error) {
	if page < 1 {
		return nil, errors.New("页码必须大于零")
	}
	b, e := dashboardPage(d.client, fmt.Sprintf("%s?page=%d&per-page=10", path, page))
	if e != nil {
		return nil, e
	}
	doc, e := pageDOM(b)
	if e != nil {
		return nil, e
	}
	titles := nodes(doc, "title")
	if len(titles) != 1 || nodeText(titles[0]) != title {
		return nil, pageFormatError()
	}
	for _, t := range nodes(doc, "table") {
		rows := tableRows(t)
		if len(rows) > 0 {
			return rows, nil
		}
		if explicitEmpty(t) {
			return []gridRow{}, nil
		}
	}
	return nil, pageFormatError()
}
func (d *DashboardHandler) GetBill(page int) ([]BillRecord, error) {
	rows, e := d.records("/log/check-out", "结算清单", page)
	if e != nil {
		return nil, e
	}
	out := []BillRecord{}
	for _, r := range rows {
		id, f, v, traffic, duration, date := field(r, "0", "编号"), field(r, "2", "固定费用"), field(r, "3", "实时费用"), field(r, "7", "使用流量"), field(r, "10", "使用时长"), field(r, "12", "结算时间")
		if !required(id, f, v, traffic, duration, date) {
			return nil, pageFormatError()
		}
		fixed, e1 := strconv.ParseFloat(f, 64)
		variable, e2 := strconv.ParseFloat(v, 64)
		if e1 != nil || e2 != nil {
			return nil, errors.New("账单金额无效")
		}
		out = append(out, BillRecord{id, fixed + variable, traffic, duration, date})
	}
	return out, nil
}
func (d *DashboardHandler) GetUsageRecords(page int) ([]UsageRecord, error) {
	rows, e := d.records("/log/detail", "上网明细", page)
	if e != nil {
		return nil, e
	}
	out := []UsageRecord{}
	for _, r := range rows {
		v := UsageRecord{field(r, "1", "上线时间"), field(r, "2", "下线时间"), field(r, "5", "IP地址"), field(r, "17", "使用流量"), field(r, "19", "使用时长")}
		if !required(v.StartTime, v.EndTime, v.IP, v.Traffic, v.UsedDuration) {
			return nil, pageFormatError()
		}
		out = append(out, v)
	}
	return out, nil
}
func (d *DashboardHandler) GetRecharge(page int) ([]RechargeRecord, error) {
	rows, e := d.records("/log/pay", "缴费清单", page)
	if e != nil {
		return nil, e
	}
	out := []RechargeRecord{}
	for _, r := range rows {
		v := RechargeRecord{field(r, "0", "编号"), field(r, "2", "缴费金额"), field(r, "6", "缴费时间")}
		if !required(v.ID, v.Cost, v.Time) {
			return nil, pageFormatError()
		}
		out = append(out, v)
	}
	return out, nil
}
