package financials

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type analysisSource struct {
	Title string `yaml:"title" json:"title"`
	URL   string `yaml:"url" json:"url"`
	Where string `yaml:"where" json:"locator"`
}

type analysisObservation struct {
	Key     string   `yaml:"key" json:"key"`
	Period  string   `yaml:"period" json:"period"`
	Basis   string   `yaml:"basis" json:"basis"`
	Unit    string   `yaml:"unit" json:"unit"`
	Value   *float64 `yaml:"value" json:"value"`
	Source  string   `yaml:"source" json:"source"`
	Subject string   `yaml:"subject" json:"subject"`
}

type analysisAssessment struct {
	Status  string   `yaml:"status" json:"status"`
	Finding string   `yaml:"finding" json:"finding"`
	Gap     string   `yaml:"gap" json:"gap,omitempty"`
	Sources []string `yaml:"sources" json:"sources"`
}

type analysisEvidence struct {
	Hash         string                        `yaml:"catalog_hash"`
	Sources      map[string]analysisSource     `yaml:"sources"`
	Observations []analysisObservation         `yaml:"observations"`
	Rules        map[string]analysisAssessment `yaml:"rules"`
	Sections     []analysisSection             `yaml:"sections"`
	HistoryNotes map[string]string             `yaml:"history_notes"`
}

type analysisSection struct {
	Title              string `yaml:"title" json:"title"`
	analysisAssessment `yaml:",inline"`
}

type analysisRule struct {
	Number int          `json:"number"`
	Rule   CatalogEntry `json:"rule"`
	analysisAssessment
	Metric *Metric `json:"metric,omitempty"`
}

type analysisPeriod struct {
	Period string            `json:"period"`
	Basis  string            `json:"basis"`
	Facts  map[string]Fact   `json:"facts"`
	Values map[string]Metric `json:"values"`
	Note   string            `json:"note,omitempty"`
}

type AnalysisReport struct {
	CatalogHash  string                    `json:"catalog_hash"`
	ReviewedHash string                    `json:"reviewed_catalog_hash"`
	Annual       []analysisPeriod          `json:"history"`
	Interim      []analysisPeriod          `json:"interim"`
	Rules        []analysisRule            `json:"rules"`
	Sections     []analysisSection         `json:"sections"`
	Sources      map[string]analysisSource `json:"sources"`
	Observations []analysisObservation     `json:"observations"`
	Results      map[string]Metric         `json:"results"`
	PeerRows     []analysisPeerRow         `json:"peer_comparisons"`
}

var historyKeys = []string{"total_assets", "common_shareholders_equity", "shareholders_equity", "net_earnings",
	"net_income_available_to_common_shareholders", "reported_earnings_per_share"}

func BuildAnalysis(ctx context.Context, db *sql.DB, catalogPath, evidencePath string) (*AnalysisReport, error) {
	catalog, hash, err := LoadCatalog(catalogPath)
	if err != nil {
		return nil, err
	}
	evidence, err := readAnalysisEvidence(evidencePath)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var storedHash string
	if err := tx.QueryRowContext(ctx, "SELECT catalog_hash FROM fundamental_entities LIMIT 1").Scan(&storedHash); err != nil {
		return nil, fmt.Errorf("load analysis database catalog: %w", err)
	}
	if storedHash != hash {
		return nil, fmt.Errorf("analysis database catalog differs from the YAML; run bbca-catalog against the analysis database")
	}
	approved, err := listApprovedFacts(ctx, tx, "")
	if err != nil {
		return nil, err
	}
	if len(approved) == 0 {
		return nil, fmt.Errorf("analysis database has no reviewed facts; see data/README.md for setup")
	}
	facts := make(map[string][]periodFact)
	for _, fact := range approved {
		if n, ok := parseCanonicalNumber(fact.NormalizedValue); ok {
			facts[fact.Period] = append(facts[fact.Period], periodFact{Fact: fact, Number: n, Sources: []Fact{fact}})
		}
	}
	report := &AnalysisReport{
		CatalogHash: hash, ReviewedHash: evidence.Hash, Sections: evidence.Sections,
		Sources: evidence.Sources, Observations: evidence.Observations,
	}
	for year := 1998; year <= 2025; year++ {
		period := strconv.Itoa(year)
		row, err := analysisPeriodFor(ctx, tx, period, "consolidated", facts[period], evidence.HistoryNotes[period])
		if err != nil {
			return nil, err
		}
		report.Annual = append(report.Annual, row)
	}
	for _, period := range []string{"2025-09", "2026-09", "2026-10"} {
		basis := "consolidated"
		if period == "2026-10" {
			basis = "unconsolidated"
		}
		row, err := analysisPeriodFor(ctx, tx, period, basis, facts[period], evidence.HistoryNotes[period])
		if err != nil {
			return nil, err
		}
		report.Interim = append(report.Interim, row)
	}
	report.PeerRows = analysisPeers(evidence.Observations)

	fy25 := report.Annual[len(report.Annual)-1]
	fy24 := report.Annual[len(report.Annual)-2]
	report.Results = analysisResults(fy24, fy25, evidence.Observations, facts)
	if evidence.Hash == hash && len(evidence.Rules) != len(catalog) {
		return nil, fmt.Errorf("pinned report requires %d rule assessments; got %d", len(catalog), len(evidence.Rules))
	}
	for i, entry := range catalog {
		assessment, ok := evidence.Rules[entry.ID]
		if !ok {
			if hash == evidence.Hash {
				return nil, fmt.Errorf("pinned report lacks assessment for %s", entry.ID)
			}
			assessment = analysisAssessment{Status: "unreviewed_rule", Finding: "This entry has no FY2025 assessment in the reviewed evidence pack."}
		}
		rule := analysisRule{Number: i + 1, Rule: entry, analysisAssessment: assessment}
		if metric, exists := report.Results[entry.ID]; entry.Type == "formula" && ok && exists {
			rule.Metric = &metric
			rule.Status = metric.Status
		}
		report.Rules = append(report.Rules, rule)
	}
	return report, tx.Commit()
}

func readAnalysisEvidence(path string) (analysisEvidence, error) {
	var evidence analysisEvidence
	file, err := os.Open(path)
	if err != nil {
		return evidence, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&evidence); err != nil {
		return evidence, fmt.Errorf("read analysis evidence: %w", err)
	}
	if extra := decoder.Decode(new(any)); extra != io.EOF {
		return evidence, fmt.Errorf("analysis evidence must contain one YAML document")
	}
	for key, source := range evidence.Sources {
		if source.Title == "" || source.Where == "" || !strings.HasPrefix(source.URL, "https://") {
			return evidence, fmt.Errorf("incomplete source %s", key)
		}
	}
	validate := func(name string, entry analysisAssessment) error {
		if entry.Status == "" || entry.Finding == "" || len(entry.Sources) == 0 {
			return fmt.Errorf("incomplete assessment %s", name)
		}
		for _, source := range entry.Sources {
			if _, ok := evidence.Sources[source]; !ok {
				return fmt.Errorf("%s references unknown source %s", name, source)
			}
		}
		return nil
	}
	for id, entry := range evidence.Rules {
		if err := validate(id, entry); err != nil {
			return evidence, err
		}
	}
	for _, section := range evidence.Sections {
		if err := validate(section.Title, section.analysisAssessment); err != nil {
			return evidence, err
		}
	}
	seen := make(map[string]bool)
	for _, item := range evidence.Observations {
		key := strings.Join([]string{item.Key, item.Subject, item.Period, item.Basis}, "/")
		if _, ok := evidence.Sources[item.Source]; !ok || seen[key] || item.Value == nil ||
			math.IsNaN(*item.Value) || math.IsInf(*item.Value, 0) || item.Key == "" ||
			item.Period == "" || item.Basis == "" || item.Unit == "" || item.Subject == "" {
			return evidence, fmt.Errorf("duplicate or invalid observation %s", key)
		}
		seen[key] = true
	}
	return evidence, nil
}

func analysisPeriodFor(ctx context.Context, tx *sql.Tx, period, basis string, facts []periodFact, note string) (analysisPeriod, error) {
	row := analysisPeriod{Period: period, Basis: basis, Facts: make(map[string]Fact), Values: make(map[string]Metric), Note: note}
	var byBasis []periodFact
	for _, fact := range facts {
		if fact.Fact.ReportingBasis == basis {
			byBasis = append(byBasis, fact)
		}
	}
	selected, conflicts := numericInputs(byBasis)
	for _, key := range historyKeys {
		if item, ok := selected[key]; ok && !conflicts[key] && item.Fact.ReportingBasis == basis {
			row.Facts[key] = item.Fact
		} else if conflicts[key] {
			row.Note = strings.TrimSpace(row.Note + " Conflicting approved " + key + ".")
		}
	}
	if basis == "unconsolidated" {
		return row, nil
	}
	metrics, err := calculateMetrics(ctx, tx, period, basis)
	if err != nil {
		return row, err
	}
	for _, metric := range metrics {
		row.Values[metric.EntityID] = metric
	}
	return row, nil
}

type analysisPeerRow struct {
	Key          string                `json:"key"`
	Basis        string                `json:"basis"`
	Status       string                `json:"status"`
	Observations []analysisObservation `json:"observations"`
}

func analysisPeers(items []analysisObservation) []analysisPeerRow {
	var rows []analysisPeerRow
	for _, key := range []string{"Gross NPL", "Capital adequacy ratio", "Loan to deposit ratio"} {
		row := analysisPeerRow{Key: key, Basis: "bank only"}
		for _, subject := range []string{"BBCA", "BBRI", "BMRI"} {
			for _, item := range items {
				if item.Period == "2025" && item.Key == key && item.Subject == subject && item.Basis == row.Basis && item.Unit == "%" {
					row.Observations = append(row.Observations, item)
				}
			}
		}
		row.Status = "comparable"
		if len(row.Observations) != 3 {
			row.Status = "matching basis or input missing"
		}
		rows = append(rows, row)
	}
	return rows
}

func derived(id, period, unit string, value float64, inputs map[string]any) Metric {
	return Metric{EntityID: id, Period: period, Unit: unit, Value: &value, Inputs: inputs, Status: "calculated"}
}

func analysisResults(fy24, fy25 analysisPeriod, items []analysisObservation, facts map[string][]periodFact) map[string]Metric {
	results := make(map[string]Metric)
	for _, rule := range []string{"earnings-per-share", "return-on-assets", "return-on-equity", "effective-tax-rate",
		"total-liabilities-to-equity", "net-change-in-cash", "capital-expenditure-to-net-earnings", "net-profit-margin"} {
		if metric, ok := fy25.Values[rule]; ok {
			results[rule] = metric
		}
	}
	obs := func(key, period, basis, unit string) (float64, any, bool) {
		for _, item := range items {
			if item.Subject == "BBCA" && item.Period == period && item.Key == key && item.Basis == basis && item.Unit == unit {
				return *item.Value, item, true
			}
		}
		return 0, nil, false
	}
	market := func(key, period, unit string) (float64, any, bool) { return obs(key, period, "market", unit) }
	close25, c25, closeOK := market("Year-end close", "2025", "IDR/share")
	close24, c24, priorOK := market("Year-end close", "2024", "IDR/share")
	paid, paidRef, paidOK := market("Cash dividends paid", "2025", "IDR/share")
	dps, dpsRef, dpsOK := market("Fiscal dividends", "2025", "IDR/share")
	shares, sharesRef, sharesOK := obs("Closing common shares ex treasury", "2025", "consolidated", "shares")
	inflation, inflationRef, inflationOK := obs("CPI growth", "2025", "Indonesia CPI", "%")
	earnings := results["earnings-per-share"]
	equity, equityOK := fy25.Facts["common_shareholders_equity"]
	book, bookOK := parseCanonicalNumber(equity.NormalizedValue)
	scale, scaleOK := unitScale(equity.Unit)
	for id, reason := range map[string]string{
		"price-earnings-ratio":        "Requires a positive year-end close and approved FY2025 EPS.",
		"price-to-book-ratio":         "Requires a positive year-end close, approved consolidated common equity, and closing shares excluding treasury stock.",
		"dividend-yield":              "Requires a positive year-end close and cash dividends paid during 2025.",
		"real-return-after-inflation": "Requires both year-end closes, matching calendar dividends, and matching-period national CPI growth.",
		"nominal-shareholder-return":  "Requires both year-end closes and matching calendar dividends.",
		"fiscal-dividend-payout":      "Requires confirmed fiscal dividends and approved FY2025 EPS.",
		"margin-of-safety":            "Requires a positive year-end close and a conservative dividend value.",
	} {
		results[id] = Metric{EntityID: id, Period: "2025", Status: "missing_inputs", Reason: reason}
	}
	for _, scenario := range dividendScenarios {
		id := "dividend-value-" + scenario.name
		results[id] = Metric{EntityID: id, Period: "2025", Status: "missing_inputs", Reason: "Requires positive confirmed fiscal dividends."}
	}
	if closeOK && close25 > 0 {
		if earnings.Value != nil && *earnings.Value > 0 {
			results["price-earnings-ratio"] = derived("price-earnings-ratio", "2025", "×", close25 / *earnings.Value, map[string]any{"close": c25, "eps": earnings})
		}
		if equityOK && bookOK && scaleOK && book > 0 && sharesOK && shares > 0 {
			bvps := book * scale / shares
			results["price-to-book-ratio"] = derived("price-to-book-ratio", "2025", "×", close25/bvps, map[string]any{"close": c25, "common_equity": equity, "shares": sharesRef, "book_value_per_share": bvps})
		}
		if paidOK && paid >= 0 {
			results["dividend-yield"] = derived("dividend-yield", "2025", "%", paid/close25*100, map[string]any{"close": c25, "cash_dividends_paid": paidRef})
		}
		if priorOK && close24 > 0 && paidOK && paid >= 0 {
			nominal := (close25+paid)/close24 - 1
			results["nominal-shareholder-return"] = derived("nominal-shareholder-return", "2025", "%", nominal*100, map[string]any{"opening_close": c24, "ending_close": c25, "cash_dividends_paid": paidRef})
			if inflationOK && inflation > -100 {
				results["real-return-after-inflation"] = derived("real-return-after-inflation", "2025", "%", ((1+nominal)/(1+inflation/100)-1)*100, map[string]any{
					"nominal_return": results["nominal-shareholder-return"], "inflation": inflationRef})
			}
		}
	}
	if dpsOK && dps > 0 {
		if earnings.Value != nil && *earnings.Value > 0 {
			results["fiscal-dividend-payout"] = derived("fiscal-dividend-payout", "2025", "%", dps / *earnings.Value * 100, map[string]any{"DPS": dpsRef, "EPS": earnings})
		}
		for _, scenario := range dividendScenarios {
			value := dividendValue(dps, scenario.growth, scenario.terminal, scenario.discount)
			id := "dividend-value-" + scenario.name
			results[id] = derived(id, "2025", "IDR/share", value, map[string]any{
				"fiscal_dividends": dpsRef, "years": 5, "growth": scenario.growth, "terminal_growth": scenario.terminal, "cost_of_equity": scenario.discount})
			if scenario.name == "conservative" && closeOK && close25 > 0 {
				results["margin-of-safety"] = derived("margin-of-safety", "2025", "%", (1-close25/value)*100,
					map[string]any{"close": c25, "conservative_value": results[id]})
			}
		}
		base := dividendScenarios[1]
		for _, r := range []float64{0.10, 0.11, 0.12} {
			for _, g := range []float64{0.02, 0.03, 0.04} {
				id := fmt.Sprintf("sensitivity-%.0f-%.0f", r*100, g*100)
				results[id] = derived(id, "2025", "IDR/share", dividendValue(dps, base.growth, g, r), map[string]any{"cost_of_equity": r, "terminal_growth": g, "fiscal_dividends": dpsRef})
			}
		}
	}
	for _, row := range []analysisPeriod{fy24, fy25} {
		period := row.Period
		nii, niiRef, niiOK := obs("Net interest and sharia income", period, "consolidated", "IDR million")
		other, otherRef, otherOK := obs("Other operating income", period, "consolidated", "IDR million")
		net, netOK := row.Facts["net_earnings"]
		netNumber, numberOK := parseCanonicalNumber(net.NormalizedValue)
		if niiOK && otherOK && netOK && numberOK && net.Unit == "millions" && nii+other > 0 {
			id := "bank-net-income-to-operating-income"
			results[id+"-"+period] = derived(id, period, "%", netNumber/(nii+other)*100,
				map[string]any{"net_income": net, "net_interest_and_sharia_income": niiRef, "other_operating_income": otherRef})
		}
		start, startRef, startOK := obs("Opening cash equivalents", period, "consolidated", "IDR million")
		fx, fxRef, fxOK := obs("FX effect on cash", period, "consolidated", "IDR million")
		end, endRef, endOK := obs("Closing cash equivalents", period, "consolidated", "IDR million")
		cash := row.Values["net-change-in-cash"]
		if startOK && fxOK && endOK && cash.Value != nil && cash.Unit == "IDR millions" {
			id := "cash-reconciliation-residual"
			results[id+"-"+period] = derived(id, period, "IDR million", start+*cash.Value+fx-end,
				map[string]any{"opening": startRef, "cash_subtotal": cash, "fx_effect": fxRef, "closing": endRef})
		}
		selected, conflicts := numericInputs(facts[period])
		if pbt, ok := selected["income_before_tax"]; ok && !conflicts["income_before_tax"] && pbt.Fact.Unit == "millions" && pbt.Fact.ReportingBasis == "consolidated" {
			insurance, insRef, insOK := obs("Net insurance income", period, "consolidated", "IDR million")
			provision, provisionRef, provisionOK := obs("Impairment losses", period, "consolidated", "IDR million")
			expense, expOK := selected["operating_expenses"]
			if niiOK && otherOK && insOK && provisionOK && expOK && !conflicts["operating_expenses"] &&
				expense.Fact.Unit == "millions" && expense.Fact.ReportingBasis == pbt.Fact.ReportingBasis {
				id := "pretax-bridge-residual"
				results[id+"-"+period] = derived(id, period, "IDR million", nii+other+insurance-provision+expense.Number-pbt.Number,
					map[string]any{"nii": niiRef, "other_income": otherRef, "insurance": insRef, "impairment": provisionRef, "expenses": expense.Fact, "pretax": pbt.Fact})
			}
		}
	}
	return results
}

var dividendScenarios = []struct {
	name                       string
	growth, terminal, discount float64
}{
	{"conservative", .03, .02, .12},
	{"base", .06, .03, .11},
	{"favorable", .09, .04, .10},
}

func dividendValue(dps, growth, terminal, discount float64) float64 {
	value := 0.0
	for year := 1; year <= 5; year++ {
		dps *= 1 + growth
		value += dps / math.Pow(1+discount, float64(year))
	}
	return value + dps*(1+terminal)/(discount-terminal)/math.Pow(1+discount, 5)
}

func historyFact(fact Fact, money bool) string {
	n, ok := parseCanonicalNumber(fact.NormalizedValue)
	if !ok {
		return "—"
	}
	if money {
		if scale, ok := unitScale(fact.Unit); ok {
			n = n * scale / 1e12
		} else {
			return "—"
		}
	}
	return fmt.Sprintf("[%.2f](%s#page=%d)", n, fact.SourceURL, fact.PageNumber)
}

func percentMetric(metric Metric) string {
	if metric.Value == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *metric.Value)
}

func displayKey(s string) string {
	return strings.ReplaceAll(s, "-", " ")
}
