package model

import (
	"testing"
	"time"
)

func TestParseTrafficUnitsAndPrecision(t *testing.T) {
	for _, tc := range []struct {
		display           string
		base              int
		bytes, resolution int64
	}{
		{"1.25 GB", 1000, 1250000000, 10000000},
		{"1.25 GiB", 0, 1342177280, 10737419},
		{"123456789012345 B", 0, 123456789012345, 1},
		{"0 B", 0, 0, 1},
		{"1024 KB", 1024, 1048576, 1024},
	} {
		m, err := ParseTraffic(tc.display, tc.base)
		if err != nil || m.Bytes != tc.bytes || m.ResolutionBytes != tc.resolution {
			t.Errorf("%s: got %+v, %v", tc.display, m, err)
		}
	}
	for _, display := range []string{"1 GB", "NaN B", "-2 B", "1 Gb/s", "999999999999999999999 TB", "无数据"} {
		if _, err := ParseTraffic(display, 0); err == nil {
			t.Errorf("accepted invalid or ambiguous value %q", display)
		}
	}
	if _, err := ParseTraffic("1 Gb", 1000); err == nil {
		t.Fatal("bit/byte ambiguity must be rejected")
	}
}

func sampleSnapshot() Snapshot {
	m, _ := ParseTraffic("1.00 GB", 1000)
	return Snapshot{SchemaVersion: 1, AccountID: "test-user", CapturedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), QueryStatus: "ok", Source: "ipgw-dashboard", BillingPeriod: "2026-09", PeriodSource: "user", Traffic: m}
}

func TestCompareSnapshots(t *testing.T) {
	before := sampleSnapshot()
	after := before
	after.CapturedAt = before.CapturedAt.Add(time.Minute)
	result, err := CompareSnapshots(before, after)
	if err != nil || result.Result != "display_unchanged" || result.DeltaBytes != 0 || result.Note == "" {
		t.Fatalf("unchanged display: %+v %v", result, err)
	}
	after.Traffic, _ = ParseTraffic("1.02 GB", 1000)
	result, err = CompareSnapshots(before, after)
	if err != nil || result.DeltaBytes != 20000000 || result.UncertaintyBytes != 20000000 {
		t.Fatalf("delta: %+v %v", result, err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"different account", func(s *Snapshot) { s.AccountID = "other" }},
		{"new cycle", func(s *Snapshot) { s.BillingPeriod = "2026-10" }},
		{"unknown cycle", func(s *Snapshot) { s.BillingPeriod = ""; s.PeriodSource = "unknown" }},
		{"failed query", func(s *Snapshot) { s.QueryStatus = "error" }},
		{"time reversed", func(s *Snapshot) { s.CapturedAt = before.CapturedAt }},
		{"counter reset", func(s *Snapshot) { s.Traffic, _ = ParseTraffic("0.50 GB", 1000) }},
		{"tampered counter", func(s *Snapshot) { s.Traffic.Bytes++ }},
		{"base mismatch", func(s *Snapshot) { s.Traffic, _ = ParseTraffic("1.02 GB", 1024) }},
		{"unknown schema", func(s *Snapshot) { s.SchemaVersion = 9 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := after
			tc.mutate(&s)
			if _, err := CompareSnapshots(before, s); err == nil {
				t.Fatal("unsafe comparison accepted")
			}
		})
	}
}

func TestCompareByteCounterToBinaryUnit(t *testing.T) {
	before := sampleSnapshot()
	before.Traffic, _ = ParseTraffic("0 B", 0)
	after := before
	after.CapturedAt = before.CapturedAt.Add(time.Minute)
	after.Traffic, _ = ParseTraffic("1 KiB", 0)
	result, err := CompareSnapshots(before, after)
	if err != nil || result.DeltaBytes != 1024 {
		t.Fatalf("byte units must be base-neutral: %+v %v", result, err)
	}
}
