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

func TestPeriodsForExplicitColumns(t *testing.T) {
	highlights := []string{"Five-year highlights", "1998        1999        2000        2001        2002", "Total assets 100 200 300 400 500"}
	got := periodsForColumns(highlights, 2, "2002", 5)
	want := []string{"1998", "1999", "2000", "2001", "2002"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("period[%d] = %q, want %q", index, got[index], want[index])
		}
	}

	segments := []string{"2002", "Interest income 100 200 300 400 500 600 2100"}
	got = periodsForColumns(segments, 1, "2002", 7)
	for index, period := range got {
		if period != "2002" {
			t.Fatalf("segment period[%d] = %q, want 2002", index, period)
		}
	}

	interim := []string{"2026          2025", "Total assets 100 90"}
	got = periodsForColumns(interim, 1, "2026-09", 2)
	if got[0] != "2026-09" || got[1] != "" {
		t.Fatalf("interim comparative periods = %#v, want current interim and unknown comparative", got)
	}

	ambiguous := []string{"2025          2024", "Net profit 100 90 80"}
	got = periodsForColumns(ambiguous, 1, "2025", 3)
	for _, period := range got {
		if period != "" {
			t.Fatalf("mismatched column count assigned period %q", period)
		}
	}
}

func TestPageBasisMixedHeadingsStaysUnknown(t *testing.T) {
	if got := pageBasis("consolidated and unconsolidated statements"); got != "unknown" {
		t.Fatalf("pageBasis mixed heading = %q, want unknown", got)
	}
}

func TestDetectCanonicalKeyRecognizesFixedAssetAcquisitions(t *testing.T) {
	for _, label := range []string{
		"Acquisition of fixed assets (1,234)",
		"Acquisition of property and equipment (1,234)",
		"Perolehan aset tetap (1.234)",
	} {
		if got := detectCanonicalKey(label); got != "capital_expenditures" {
			t.Errorf("detectCanonicalKey(%q) = %q, want capital_expenditures", label, got)
		}
	}
}

func TestComparativeFactPeriodsKeepSourcePage(t *testing.T) {
	document := importDocument{Document: Document{SHA256: "document-hash", Period: "2008"}}
	page := Page{Number: 42, Text: "2008             2007\nTotal assets       1.250.000       1.100.000", ExtractionMethod: "pdftotext"}
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
	if facts[0].SourceLocator != "page:42" || facts[1].SourceLocator != "page:42" {
		t.Fatalf("source locators = %q, %q, want page:42", facts[0].SourceLocator, facts[1].SourceLocator)
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

func TestMetricInputsRequireKnownMetadata(t *testing.T) {
	tests := []struct {
		name string
		fact Fact
	}{
		{name: "unknown basis", fact: Fact{ReportingBasis: "unknown", AuditedStatus: "audited", Currency: "IDR", Unit: "millions"}},
		{name: "unknown audit status", fact: Fact{ReportingBasis: "consolidated", AuditedStatus: "unknown", Currency: "IDR", Unit: "millions"}},
		{name: "unknown currency", fact: Fact{ReportingBasis: "consolidated", AuditedStatus: "audited", Unit: "millions"}},
		{name: "unknown units", fact: Fact{ReportingBasis: "consolidated", AuditedStatus: "audited", Currency: "IDR"}},
		{name: "generic units", fact: Fact{ReportingBasis: "consolidated", AuditedStatus: "audited", Currency: "IDR", Unit: "as displayed"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]periodFact{
				"net_earnings": {Fact: test.fact},
				"revenue":      {Fact: test.fact},
			}
			if reason := metricInputCompatibility(values, []string{"net_earnings", "revenue"}, "net-profit-margin"); reason == "" {
				t.Fatal("metric accepted unknown or generic metadata")
			}
		})
	}
	valid := Fact{ReportingBasis: "consolidated", AuditedStatus: "audited", Currency: "IDR", Unit: "millions"}
	values := map[string]periodFact{"net_earnings": {Fact: valid}, "revenue": {Fact: valid}}
	if reason := metricInputCompatibility(values, []string{"net_earnings", "revenue"}, "net-profit-margin"); reason != "" {
		t.Fatalf("compatible known metadata rejected: %s", reason)
	}
}

func TestFormulaDispositionClassifiesNonCalculatedCatalogEntries(t *testing.T) {
	want := map[string]string{
		"closed-end-fund-discount-to-nav": "not_applicable",
		"dividend-yield":                  "requires_external_data",
		"earnings-yield-on-original-cost": "requires_external_data",
		"insurance-float-cost":            "not_applicable",
		"look-through-earnings":           "requires_external_data",
		"owner-earnings":                  "requires_manual_estimate",
		"price-earnings-ratio":            "requires_external_data",
		"price-to-book-ratio":             "requires_external_data",
		"real-return-after-inflation":     "requires_external_data",
	}
	for id, wantStatus := range want {
		status, reason, ok := formulaDisposition(id)
		if !ok || status != wantStatus || reason == "" {
			t.Errorf("formulaDisposition(%q) = (%q, %q, %t), want status %q with reason", id, status, reason, ok, wantStatus)
		}
	}
}
