package model

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// Snapshot contains only confirmed successful queries. Display counters are
// rounded by the server; neither equal snapshots nor a zero delta prove free traffic.
type Snapshot struct {
	SchemaVersion int         `json:"schema_version"`
	AccountID     string      `json:"account_id"`
	CapturedAt    time.Time   `json:"captured_at"`
	QueryStatus   string      `json:"query_status"`
	Source        string      `json:"source"`
	BillingPeriod string      `json:"billing_period"`
	PeriodSource  string      `json:"period_source"`
	Traffic       Measurement `json:"traffic"`
}

type Measurement struct {
	Display         string `json:"display"`
	Unit            string `json:"unit"`
	Base            int    `json:"base"`
	Bytes           int64  `json:"approximate_bytes"`
	ResolutionBytes int64  `json:"display_resolution_bytes"`
}

var trafficRE = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(B|BYTE|BYTES|KB|MB|GB|TB|KIB|MIB|GIB|TIB|K|M|G|T)\s*$`)

func ParseTraffic(display string, base int) (Measurement, error) {
	m := Measurement{Display: display}
	if base != 0 && base != 1000 && base != 1024 {
		return m, errors.New("流量单位基数只能是 1000 或 1024")
	}
	parts := trafficRE.FindStringSubmatch(display)
	if parts == nil {
		return m, fmt.Errorf("无法识别流量单位：%q", display)
	}
	if len(parts[2]) > 1 && strings.HasSuffix(parts[2], "b") {
		return m, errors.New("小写 b 可能表示比特，不能按字节生成快照；请确认页面单位")
	}
	unit := strings.ToUpper(parts[2])
	power := strings.Index("KMGT", unit[:1]) + 1
	if strings.HasPrefix(unit, "B") {
		power, base = 0, 1000
	} else if strings.Contains(unit, "IB") {
		base = 1024
	} else if base == 0 {
		return m, errors.New("页面单位未说明 1000/1024 基数，请核实后用 --traffic-base 指定；不能凭显示值猜测字节数")
	}
	value, ok := new(big.Rat).SetString(parts[1])
	if !ok {
		return m, errors.New("流量数值无效")
	}
	multiplier := new(big.Int).Exp(big.NewInt(int64(base)), big.NewInt(int64(power)), nil)
	value.Mul(value, new(big.Rat).SetInt(multiplier))
	bytes := new(big.Int).Quo(value.Num(), value.Denom())
	if !bytes.IsInt64() {
		return m, errors.New("流量数值超出范围")
	}
	digits := 0
	if dot := strings.IndexByte(parts[1], '.'); dot >= 0 {
		digits = len(parts[1]) - dot - 1
	}
	if digits > 12 {
		return m, errors.New("流量小数位数超出支持范围")
	}
	denom := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	resolution, remainder := new(big.Int), new(big.Int)
	resolution.QuoRem(multiplier, denom, remainder)
	if remainder.Sign() > 0 {
		resolution.Add(resolution, big.NewInt(1))
	}
	if !resolution.IsInt64() || resolution.Sign() <= 0 {
		return m, errors.New("流量精度无效")
	}
	m.Unit, m.Base, m.Bytes, m.ResolutionBytes = unit, base, bytes.Int64(), resolution.Int64()
	return m, nil
}

type SnapshotComparison struct {
	AccountID        string    `json:"account_id"`
	BillingPeriod    string    `json:"billing_period"`
	Before           time.Time `json:"before"`
	After            time.Time `json:"after"`
	DeltaBytes       int64     `json:"approximate_delta_bytes"`
	UncertaintyBytes int64     `json:"display_uncertainty_bytes"`
	Result           string    `json:"result"`
	Note             string    `json:"note"`
}

func validateSnapshot(s Snapshot) error {
	if s.SchemaVersion != 1 || s.QueryStatus != "ok" || s.AccountID == "" || s.CapturedAt.IsZero() || s.Source != "ipgw-dashboard" {
		return errors.New("快照格式无效或查询未成功")
	}
	if s.BillingPeriod == "" || (s.PeriodSource != "server" && s.PeriodSource != "user") {
		return errors.New("快照计费周期未知，不能进行可靠对比")
	}
	m, err := ParseTraffic(s.Traffic.Display, s.Traffic.Base)
	if err != nil || m != s.Traffic {
		return errors.New("快照流量数值、单位或精度不一致")
	}
	return nil
}

func CompareSnapshots(before, after Snapshot) (*SnapshotComparison, error) {
	if err := validateSnapshot(before); err != nil {
		return nil, fmt.Errorf("前快照：%w", err)
	}
	if err := validateSnapshot(after); err != nil {
		return nil, fmt.Errorf("后快照：%w", err)
	}
	if before.AccountID != after.AccountID {
		return nil, errors.New("不能对比不同账号的快照")
	}
	if before.BillingPeriod != after.BillingPeriod || before.PeriodSource != after.PeriodSource {
		return nil, errors.New("计费周期或周期来源不一致，不能计算差值")
	}
	if !after.CapturedAt.After(before.CapturedAt) {
		return nil, errors.New("后快照时间必须晚于前快照")
	}
	// Byte units are independent of the decimal/binary prefix convention.
	if before.Traffic.Base != after.Traffic.Base && !strings.HasPrefix(before.Traffic.Unit, "B") && !strings.HasPrefix(after.Traffic.Unit, "B") {
		return nil, errors.New("流量单位基数不一致")
	}
	if after.Traffic.Bytes < before.Traffic.Bytes {
		return nil, errors.New("流量计数下降，可能发生重置或周期变化，不能用于免流判断")
	}
	if before.Traffic.ResolutionBytes > math.MaxInt64-after.Traffic.ResolutionBytes {
		return nil, errors.New("快照精度超出范围")
	}
	result := &SnapshotComparison{
		AccountID: before.AccountID, BillingPeriod: before.BillingPeriod,
		Before: before.CapturedAt, After: after.CapturedAt,
		DeltaBytes:       after.Traffic.Bytes - before.Traffic.Bytes,
		UncertaintyBytes: before.Traffic.ResolutionBytes + after.Traffic.ResolutionBytes,
		Result:           "display_increased",
		Note:             "仅比较服务器显示的计费用量；统计延迟、显示舍入和其他设备流量均会影响结果，不能单凭差值确认免流。",
	}
	if result.DeltaBytes == 0 {
		result.Result = "display_unchanged"
	}
	if before.PeriodSource == "user" {
		result.Note += " 计费周期由用户声明，未由服务器验证。"
	}
	return result, nil
}
