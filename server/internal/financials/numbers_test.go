package financials

import "testing"

func TestParseNumber(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want float64
		ok   bool
	}{
		{name: "Indonesian thousands and decimal", raw: "1.234.567,89", want: 1234567.89, ok: true},
		{name: "English thousands and decimal", raw: "1,234,567.89", want: 1234567.89, ok: true},
		{name: "English grouped integer", raw: "1,234,567", want: 1234567, ok: true},
		{name: "Indonesian grouped integer", raw: "1.234.567", want: 1234567, ok: true},
		{name: "parenthesized negative", raw: "(1.234,50)", want: -1234.5, ok: true},
		{name: "rupiah", raw: "Rp 1.234,50", want: 1234.5, ok: true},
		{name: "dash is missing", raw: "-", ok: false},
		{name: "long dash is missing", raw: "—", ok: false},
		{name: "not meaningful is missing", raw: "n.m.", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseNumber(test.raw)
			if ok != test.ok || (ok && got != test.want) {
				t.Fatalf("ParseNumber(%q) = (%v, %v), want (%v, %v)", test.raw, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestComparativePeriods(t *testing.T) {
	got := comparativePeriods("2008", 3)
	want := []string{"2008", "2007", "2006"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("period[%d] = %q, want %q", index, got[index], want[index])
		}
	}
	interim := comparativePeriods("2026-09", 2)
	if interim[0] != "2026-09" || interim[1] != "" {
		t.Fatalf("interim comparative periods = %#v, want current period only", interim)
	}
}

func TestPageBasisMixedHeadingsStaysUnknown(t *testing.T) {
	if got := pageBasis("consolidated and unconsolidated statements"); got != "unknown" {
		t.Fatalf("pageBasis mixed heading = %q, want unknown", got)
	}
}

func TestComparativeFactPeriodsKeepSourcePage(t *testing.T) {
	document := importDocument{Document: Document{SHA256: "document-hash", Period: "2008"}}
	page := Page{Number: 42, Text: "Total assets       1.250.000       1.100.000", ExtractionMethod: "pdftotext"}
	facts := candidatesFromPage(document, page)
	if len(facts) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(facts))
	}
	if facts[0].Period != "2008" || facts[1].Period != "2007" {
		t.Fatalf("candidate periods = %q, %q, want 2008 and 2007", facts[0].Period, facts[1].Period)
	}
	if facts[0].PageNumber != 42 || facts[1].PageNumber != 42 {
		t.Fatalf("source page numbers = %d, %d, want 42", facts[0].PageNumber, facts[1].PageNumber)
	}
}

func TestMetricInputsRejectMixedReportingBasis(t *testing.T) {
	values := map[string]periodFact{
		"net_earnings": {Fact: Fact{ReportingBasis: "consolidated", AuditedStatus: "audited", Currency: "IDR", Unit: "millions"}},
		"revenue":      {Fact: Fact{ReportingBasis: "unconsolidated", AuditedStatus: "audited", Currency: "IDR", Unit: "millions"}},
	}
	if reason := metricInputCompatibility(values, []string{"net_earnings", "revenue"}, "net-profit-margin"); reason == "" {
		t.Fatal("mixed reporting basis was allowed")
	}
}
