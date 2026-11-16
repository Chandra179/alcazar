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

func TestStatementParserSeparatesComparativeColumnsAndLabels(t *testing.T) {
	document := importDocument{Document: Document{SHA256: "hash", Period: "2025", Audited: true}}
	page := Page{Number: 8, Text: `PT BANK CENTRAL ASIA Tbk AND SUBSIDIARIES
CONSOLIDATED STATEMENTS OF FINANCIAL POSITION
31 DECEMBER 2025 AND 2024
(Expressed in millions of Rupiah)
                                               Notes       2025          2024
TOTAL LIABILITIES                               12       1,294,508      1,176,979
TOTAL EQUITY                                             281,687        262,835
TOTAL LIABILITIES AND EQUITY                            1,586,829      1,449,301`, ExtractionMethod: "pdftotext"}
	facts := parseStatements(document, []Page{page})
	if len(facts) != 4 {
		t.Fatalf("facts = %#v", facts)
	}
	if fact := facts[0]; fact.CanonicalKey != "total_liabilities" || fact.Period != "2025" || fact.NormalizedValue != "1294508" || fact.PeriodEnd != "2025-12-31" || fact.PeriodType != "instant" || fact.SourceLocator != "page:8/balance/row:6/column:1" {
		t.Fatalf("FY2025 liabilities = %#v", fact)
	}
	if facts[1].Period != "2024" || facts[1].NormalizedValue != "1176979" {
		t.Fatalf("comparative = %#v", facts[1])
	}
}

func TestParseCanonicalNumberKeepsThreeDecimalPlaces(t *testing.T) {
	if number, ok := parseCanonicalNumber("-2447.423"); !ok || number != -2447.423 {
		t.Fatalf("canonical number = %v, %t", number, ok)
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
		definition, ok := formulaDefinitions[id]
		status, reason := definition.status, definition.reason
		if !ok || status != wantStatus || reason == "" {
			t.Errorf("formulaDefinitions[%q] = (%q, %q, %t), want status %q with reason", id, status, reason, ok, wantStatus)
		}
	}
}
