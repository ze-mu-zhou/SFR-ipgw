package handler

import (
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"golang.org/x/net/html"
)

type DashboardHandler struct {
	client    *http.Client
	indexBody string
}

func NewDashboardHandler() *DashboardHandler {
	return &DashboardHandler{client: newSession()}
}

func (d *DashboardHandler) Login(account *model.Account) error {
	password, err := account.GetPassword()
	if err != nil {
		return err
	}
	if err = loginCAS(d.client, casLoginURL, account.Username, password); err != nil {
		return err
	}
	_, err = dashboardPage(d.client, "/sso/neusoft/index")
	d.indexBody = ""
	return err
}

func (d *DashboardHandler) cachedIndexBody() (string, error) {
	if d.indexBody == "" {
		body, err := dashboardPage(d.client, "/home")
		if err != nil {
			return "", err
		}
		d.indexBody = body
	}
	return d.indexBody, nil
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

// BillRecord 是一条扣费记录。金额字段统一使用十进制字符串，
// 与页面显示一致，避免浮点舍入。
type BillRecord struct {
	ID           string `json:"id"`
	Cost         string `json:"cost"`
	Traffic      string `json:"traffic"`
	UsedDuration string `json:"used_duration"`
	Date         string `json:"date"`
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
	body, err := d.cachedIndexBody()
	if err != nil {
		return nil, err
	}
	return pageDOM(body)
}

func (d *DashboardHandler) GetBasic() (*Basic, error) {
	doc, err := d.indexDOM()
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, labelNode := range nodes(doc, "label") {
		if labelNode.Parent != nil {
			label := nodeText(labelNode)
			values[label] = strings.TrimSpace(strings.TrimPrefix(nodeText(labelNode.Parent), label))
		}
	}
	if values["用户名"] == "" || values["姓名"] == "" {
		return nil, pageFormatError()
	}
	return &Basic{ID: values["用户名"], Name: values["姓名"]}, nil
}

// Read cell text from the DOM. Header names take precedence; known column IDs
// support the existing Yii grids when headers are abbreviated or absent.
type gridRow struct {
	cells map[string]string
	sid   string
}

func tableRows(table *html.Node) []gridRow {
	headers := map[string]string{}
	for i, header := range nodes(table, "th") {
		key := attr(header, "data-col-seq")
		if key == "" {
			key = strconv.Itoa(i)
		}
		headers[key] = nodeText(header)
	}
	var rows []gridRow
	for _, tr := range nodes(table, "tr") {
		if !isDataRow(tr, table) {
			continue
		}
		row := gridRow{cells: map[string]string{}, sid: attr(tr, "data-key")}
		for i, cell := range nodes(tr, "td") {
			key := attr(cell, "data-col-seq")
			if key == "" && attr(cell, "colspan") == "" {
				key = strconv.Itoa(i)
			}
			if key != "" {
				row.cells[key] = nodeText(cell)
				if headers[key] != "" {
					row.cells[headers[key]] = nodeText(cell)
				}
			}
		}
		if len(row.cells) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

func isDataRow(tr, table *html.Node) bool {
	// Yii/Kartik puts page summaries in tbody as well as tfoot. They have
	// ordinary td cells, but must not be parsed as incomplete records.
	for _, class := range strings.Fields(attr(tr, "class")) {
		switch class {
		case "kv-page-summary", "filters":
			return false
		}
	}
	for parent := tr.Parent; parent != nil && parent != table; parent = parent.Parent {
		if parent.Type == html.ElementNode {
			switch parent.Data {
			case "thead", "tfoot", "table":
				return false
			}
		}
	}
	return true
}

func field(row gridRow, columnID string, headers ...string) string {
	for _, header := range headers {
		if value := row.cells[header]; value != "" {
			return value
		}
	}
	return row.cells[columnID]
}

func required(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

// Column IDs are only meaningful within an identified table. In particular,
// device grids reuse 3/4/6/7 for unrelated values. Require semantic headers
// before applying the legacy column-ID fallback for individual cells.
func isPackageTable(table *html.Node) bool {
	var traffic, balance bool
	for _, header := range nodes(table, "th") {
		switch nodeText(header) {
		case "已用流量", "使用流量":
			traffic = true
		case "余额", "账户余额":
			balance = true
		}
	}
	return traffic && balance
}

var amountRE = regexp.MustCompile(`^[+-]?[0-9]+(?:\.([0-9]+))?$`)

// parseAmount 精确解析页面上的十进制金额，并返回小数位数。
// 只接受普通十进制写法，拒绝指数、分数、NaN、Inf 等。
func parseAmount(value string) (*big.Rat, int, error) {
	match := amountRE.FindStringSubmatch(value)
	if match == nil || len(match[1]) > 12 {
		return nil, 0, errors.New("金额无效")
	}
	amount, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, 0, errors.New("金额无效")
	}
	return amount, len(match[1]), nil
}

// addAmounts 精确相加两个十进制金额，结果保留两者中较多的小数位数。
func addAmounts(a, b string) (string, error) {
	x, xDigits, err := parseAmount(a)
	if err != nil {
		return "", err
	}
	y, yDigits, err := parseAmount(b)
	if err != nil {
		return "", err
	}
	return new(big.Rat).Add(x, y).FloatString(max(xDigits, yDigits)), nil
}

func (d *DashboardHandler) GetPackage() (*Package, error) {
	doc, err := d.indexDOM()
	if err != nil {
		return nil, err
	}
	for _, table := range nodes(doc, "table") {
		if !isPackageTable(table) {
			continue
		}
		for _, row := range tableRows(table) {
			pkg := &Package{
				UsedTraffic:  field(row, "3", "已用流量", "使用流量"),
				UsedDuration: field(row, "4", "已用时长", "使用时长"),
				PackageCost:  field(row, "6", "套餐费用", "消费"),
				Balance:      field(row, "7", "余额", "账户余额"),
			}
			if !required(pkg.UsedTraffic, pkg.UsedDuration, pkg.PackageCost, pkg.Balance) {
				continue
			}
			balance, _, err := parseAmount(pkg.Balance)
			if err != nil {
				return nil, errors.New("账单页面中的余额无效")
			}
			pkg.Overdue = balance.Sign() < 0
			pkg.BillingPeriod = field(row, "", "计费周期", "账期")
			return pkg, nil
		}
	}
	return nil, pageFormatError()
}

func hasEmptyClass(node *html.Node) bool {
	for _, class := range strings.Fields(attr(node, "class")) {
		if class == "empty" {
			return true
		}
	}
	return false
}

func explicitEmpty(table *html.Node) bool {
	for _, cell := range nodes(table, "td") {
		if hasEmptyClass(cell) {
			return true
		}
		for _, div := range nodes(cell, "div") {
			if hasEmptyClass(div) {
				return true
			}
		}
	}
	return false
}

func (d *DashboardHandler) GetDevice() ([]Device, error) {
	doc, err := d.indexDOM()
	if err != nil {
		return nil, err
	}
	devices := []Device{}
	for _, table := range nodes(doc, "table") {
		rows := tableRows(table)
		isDevices := false
		for _, header := range nodes(table, "th") {
			text := nodeText(header)
			if strings.Contains(text, "IP") || strings.Contains(text, "上线时间") {
				isDevices = true
			}
		}
		for _, row := range rows {
			if row.cells["9"] != "" && row.sid != "" {
				isDevices = true
			}
		}
		if !isDevices {
			continue
		}
		if explicitEmpty(table) {
			return devices, nil
		}
		for _, row := range rows {
			ip, start, stage := field(row, "1", "IP地址", "IP"), field(row, "3", "上线时间"), field(row, "7", "状态")
			if !required(ip, start, stage, row.sid) {
				return nil, pageFormatError()
			}
			devices = append(devices, Device{ID: len(devices), IP: ip, StartTime: start, Stage: stage, SID: row.sid})
		}
		if len(devices) == 0 {
			return nil, pageFormatError()
		}
		return devices, nil
	}
	return nil, pageFormatError()
}

func (d *DashboardHandler) records(path, title string, page int) ([]gridRow, error) {
	if page < 1 {
		return nil, errors.New("页码必须大于零")
	}
	body, err := dashboardPage(d.client, fmt.Sprintf("%s?page=%d&per-page=10", path, page))
	if err != nil {
		return nil, err
	}
	doc, err := pageDOM(body)
	if err != nil {
		return nil, err
	}
	titles := nodes(doc, "title")
	if len(titles) != 1 || nodeText(titles[0]) != title {
		return nil, pageFormatError()
	}
	for _, table := range nodes(doc, "table") {
		rows := tableRows(table)
		if len(rows) > 0 {
			return rows, nil
		}
		if explicitEmpty(table) {
			return []gridRow{}, nil
		}
	}
	return nil, pageFormatError()
}

func (d *DashboardHandler) GetBill(page int) ([]BillRecord, error) {
	rows, err := d.records("/log/check-out", "结算清单", page)
	if err != nil {
		return nil, err
	}
	bills := []BillRecord{}
	for _, row := range rows {
		id, fixedText, variableText := field(row, "0", "编号"), field(row, "2", "固定费用"), field(row, "3", "实时费用")
		traffic, duration, date := field(row, "7", "使用流量"), field(row, "10", "使用时长"), field(row, "12", "结算时间")
		if !required(id, fixedText, variableText, traffic, duration, date) {
			return nil, pageFormatError()
		}
		cost, err := addAmounts(fixedText, variableText)
		if err != nil {
			return nil, errors.New("账单金额无效")
		}
		bills = append(bills, BillRecord{ID: id, Cost: cost, Traffic: traffic, UsedDuration: duration, Date: date})
	}
	return bills, nil
}

func (d *DashboardHandler) GetUsageRecords(page int) ([]UsageRecord, error) {
	rows, err := d.records("/log/detail", "上网明细", page)
	if err != nil {
		return nil, err
	}
	usages := []UsageRecord{}
	for _, row := range rows {
		usage := UsageRecord{
			StartTime:    field(row, "1", "上线时间"),
			EndTime:      field(row, "2", "下线时间"),
			IP:           field(row, "5", "IP地址"),
			Traffic:      field(row, "17", "使用流量"),
			UsedDuration: field(row, "19", "使用时长"),
		}
		if !required(usage.StartTime, usage.EndTime, usage.IP, usage.Traffic, usage.UsedDuration) {
			return nil, pageFormatError()
		}
		usages = append(usages, usage)
	}
	return usages, nil
}

func (d *DashboardHandler) GetRecharge(page int) ([]RechargeRecord, error) {
	rows, err := d.records("/log/pay", "缴费清单", page)
	if err != nil {
		return nil, err
	}
	recharges := []RechargeRecord{}
	for _, row := range rows {
		recharge := RechargeRecord{ID: field(row, "0", "编号"), Cost: field(row, "2", "缴费金额"), Time: field(row, "6", "缴费时间")}
		if !required(recharge.ID, recharge.Cost, recharge.Time) {
			return nil, pageFormatError()
		}
		recharges = append(recharges, recharge)
	}
	return recharges, nil
}
