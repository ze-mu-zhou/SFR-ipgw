package handler

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func dashboardFixture(body string) *DashboardHandler {
	return &DashboardHandler{client: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}
}
func TestBillingFailuresAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		bad        bool
	}{
		{"explicit empty", `<title>结算清单</title><table><tr><td colspan="8"><div class="empty">没有找到数据。</div></td></tr></table>`, false},
		{"missing table", `<title>结算清单</title><p>页面更新</p>`, true},
		{"missing fields", `<title>结算清单</title><table><tr><td data-col-seq="0">1</td></tr></table>`, true},
		{"expired login", `<title>登录</title><table></table>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, e := dashboardFixture(tc.body).GetBill(1)
			if (e != nil) != tc.bad {
				t.Fatalf("error=%v", e)
			}
			if !tc.bad && len(r) != 0 {
				t.Fatal("expected empty")
			}
		})
	}
	d := &DashboardHandler{client: &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}}
	if _, e := d.GetBill(1); e == nil {
		t.Fatal("bill network error swallowed")
	}
	if _, e := d.GetUsageRecords(1); e == nil {
		t.Fatal("usage network error swallowed")
	}
}
func TestBillDOMWhitespaceAndHeaders(t *testing.T) {
	d := dashboardFixture(`<title>结算清单</title><table>
<tr><th data-col-seq="42">固定费用</th></tr>
<tr><td class="foo" data-col-seq='0'><span>123</span></td>
<td data-col-seq='42'> 5.25 </td><td data-col-seq='3'>0.75</td>
<td data-col-seq='7'> 1 GB </td><td data-col-seq='10'>3600</td><td data-col-seq='12'>2026-09-01</td></tr></table>`)
	rows, e := d.GetBill(1)
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 1 || rows[0].Cost != 6 || rows[0].Traffic != "1 GB" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestRecordsIgnorePageSummaries(t *testing.T) {
	for _, tc := range []struct {
		name, title, row string
		read             func(*DashboardHandler) (int, error)
	}{
		{"usage", "上网明细", `<td data-col-seq="1">2026-09-01 10:00:00</td><td data-col-seq="2">2026-09-01 11:00:00</td><td data-col-seq="5">192.0.2.1</td><td data-col-seq="17">1 GB</td><td data-col-seq="19">1小时</td>`, func(d *DashboardHandler) (int, error) {
			rows, err := d.GetUsageRecords(1)
			if err == nil && len(rows) == 1 && (rows[0].IP != "192.0.2.1" || rows[0].Traffic != "1 GB") {
				t.Errorf("unexpected usage: %+v", rows)
			}
			return len(rows), err
		}},
		{"bills", "结算清单", `<td data-col-seq="0">123</td><td data-col-seq="2">5.25</td><td data-col-seq="3">0.75</td><td data-col-seq="7">1 GB</td><td data-col-seq="10">3600</td><td data-col-seq="12">2026-09-01</td>`, func(d *DashboardHandler) (int, error) {
			rows, err := d.GetBill(1)
			if err == nil && len(rows) == 1 && (rows[0].ID != "123" || rows[0].Cost != 6) {
				t.Errorf("unexpected bill: %+v", rows)
			}
			return len(rows), err
		}},
		{"recharges", "缴费清单", `<td data-col-seq="0">123</td><td data-col-seq="2">10.00</td><td data-col-seq="6">2026-09-01</td>`, func(d *DashboardHandler) (int, error) {
			rows, err := d.GetRecharge(1)
			if err == nil && len(rows) == 1 && (rows[0].ID != "123" || rows[0].Cost != "10.00") {
				t.Errorf("unexpected recharge: %+v", rows)
			}
			return len(rows), err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := `<title>` + tc.title + `</title><table><tbody>`
			// Real pages append this summary inside tbody without data-col-seq.
			summary := `<tr class="warning kv-page-summary"><td>本页合计</td><td></td><td>10.00</td></tr>`
			body := prefix + `<tr data-key="123">` + tc.row + `</tr>` + summary + `</tbody></table>`
			if n, err := tc.read(dashboardFixture(body)); err != nil || n != 1 {
				t.Fatalf("records=%d error=%v", n, err)
			}
			// Skipping known summaries must not hide malformed data rows.
			body = prefix + `<tr>` + tc.row + `</tr><tr><td>broken record</td></tr>` + summary + `</tbody></table>`
			if _, err := tc.read(dashboardFixture(body)); err == nil {
				t.Fatal("malformed record was silently skipped")
			}
			body = prefix + `<tr><td colspan="20"><div class="empty">没有找到数据。</div></td></tr>` + summary + `</tbody></table>`
			if n, err := tc.read(dashboardFixture(body)); err != nil || n != 0 {
				t.Fatalf("empty records=%d error=%v", n, err)
			}
			if _, err := tc.read(dashboardFixture(prefix + summary + `</tbody></table>`)); err == nil {
				t.Fatal("summary alone was accepted as an empty result")
			}
		})
	}
}

func TestTableRowsExcludeNonDataRows(t *testing.T) {
	doc, err := pageDOM(`<table><thead><tr><th>编号</th></tr><tr><td><input></td></tr></thead>
<tbody><tr class="filters"><td><input></td></tr><tr><td>123</td></tr>
<tr class="kv-page-summary warning"><td>本页合计</td></tr></tbody>
<tfoot><tr><td>总计</td></tr></tfoot></table>`)
	if err != nil {
		t.Fatal(err)
	}
	rows := tableRows(nodes(doc, "table")[0])
	if len(rows) != 1 || field(rows[0], "0", "编号") != "123" {
		t.Fatalf("unexpected data rows: %+v", rows)
	}
}

func TestPackageRejectsMissingAndInvalidBalance(t *testing.T) {
	for _, b := range []string{`<table><tr><td data-col-seq="3">10 MB</td></tr></table>`, `<table><tr><td data-col-seq="3">10 MB</td><td data-col-seq="4">30</td><td data-col-seq="6">20</td><td data-col-seq="7">invalid</td></tr></table>`} {
		d := &DashboardHandler{cachedDashboardIndexContent: b}
		if _, e := d.GetPackage(); e == nil {
			t.Fatal("invalid package accepted")
		}
	}
}
func TestGatewayValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		bad        bool
	}{
		{"valid large counter", `{"error":"ok","user_name":"test","online_ip":"192.0.2.1","sum_bytes":9007199254740993,"sum_seconds":100,"user_balance":12.5}`, false},
		{"missing status", `{}`, true}, {"missing usage", `{"error":"ok","user_name":"test","online_ip":"192.0.2.1"}`, true},
		{"wrong type", `{"error":"ok","sum_bytes":"bad"}`, true}, {"missing account", `{"error":"ok"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewIpgwHandler()
			h.client = dashboardFixture(tc.body).client
			e := h.FetchUsageInfo()
			if (e != nil) != tc.bad {
				t.Fatalf("error=%v", e)
			}
			if !tc.bad && h.info.Traffic != 9007199254740993 {
				t.Fatal("integer precision lost")
			}
		})
	}
}
func TestParseBasicInfoPropagatesFailure(t *testing.T) {
	h := NewIpgwHandler()
	h.client = dashboardFixture(`{}`).client
	if e := h.ParseBasicInfo(); e == nil {
		t.Fatal("failure hidden")
	}
}

func TestKickInitializationCanRetry(t *testing.T) {
	h := NewIpgwHandler()
	initializations := 0
	h.client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/sso/neusoft/index":
			initializations++
			if initializations == 1 {
				return nil, errors.New("temporary outage")
			}
		case "/home":
			body = `<meta content="a+b&amp;c" name="csrf-token">`
		case "/home/delete":
			if e := r.ParseForm(); e != nil {
				t.Fatal(e)
			}
			if r.PostForm.Get("_csrf-8800") != "a+b&c" || r.URL.Query().Get("id") != "session&1" {
				t.Fatal("form values were not safely encoded")
			}
			body = "下线请求已发出"
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	if _, e := h.Kick("session&1"); e == nil {
		t.Fatal("initial failure swallowed")
	}
	if ok, e := h.Kick("session&1"); e != nil || !ok {
		t.Fatalf("retry failed: %v", e)
	}
	if initializations != 2 {
		t.Fatalf("initializations=%d", initializations)
	}
}
func TestStandardTableHeaderMapping(t *testing.T) {
	d := dashboardFixture(`<title>结算清单</title><table><tr><th>编号</th><th>固定费用</th><th>实时费用</th><th>使用流量</th><th>使用时长</th><th>结算时间</th></tr><tr><td>1</td><td>2</td><td>3</td><td>4 GB</td><td>50</td><td>2026-09-01</td></tr></table>`)
	rows, e := d.GetBill(1)
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 1 || rows[0].Cost != 5 || rows[0].Traffic != "4 GB" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestNetworkErrorsDoNotExposeTicket(t *testing.T) {
	const secret = "synthetic-secret-ticket"
	d := &DashboardHandler{client: &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: "https://example.test/?ticket=" + secret, Err: errors.New("offline")}
	})}}
	_, e := d.GetBill(1)
	if e == nil || strings.Contains(e.Error(), secret) || !strings.Contains(e.Error(), "offline") {
		t.Fatalf("unsafe or missing error: %v", e)
	}
}

func TestConnectionDistinguishesOfflineFromLoggedOut(t *testing.T) {
	for _, tc := range []struct {
		name, body                     string
		connected, loggedIn, wantError bool
	}{
		{"logged in", `{"error":"ok","user_name":"dummy","online_ip":"192.0.2.1"}`, true, true, false},
		{"logged out", `{"error":"not_online_error","client_ip":"192.0.2.1"}`, true, false, false},
		{"invalid response", `{}`, false, false, true},
		{"server error is not logged out", `{"error":"server_error","client_ip":"192.0.2.1"}`, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewIpgwHandler()
			h.client = dashboardFixture(tc.body).client
			connected, loggedIn, e := h.CheckConnection()
			if connected != tc.connected || loggedIn != tc.loggedIn || (e != nil) != tc.wantError {
				t.Fatalf("got %v,%v,%v", connected, loggedIn, e)
			}
		})
	}
	h := NewIpgwHandler()
	h.client.Transport = testTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if _, _, e := h.CheckConnection(); e == nil {
		t.Fatal("network error became disconnected success")
	}
}
