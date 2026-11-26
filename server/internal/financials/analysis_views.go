package financials

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/Chandra179/lux/server/internal/report"
)

type (
	reportLink     = report.Link
	reportCell     = report.Cell
	reportTable    = report.Table
	reportSection  = report.Section
	yearlyTemplate = report.Template
)

func textCells(values ...string) []reportCell {
	row := make([]reportCell, 0, len(values))
	for _, value := range values {
		row = append(row, reportCell{Text: value})
	}
	return row
}

func pageURL(url string, page int) string {
	if page > 0 && url != "" {
		base, _, _ := strings.Cut(url, "#")
		return fmt.Sprintf("%s#page=%d", base, page)
	}
	return url
}

func factCell(fact Fact, trillions bool) reportCell {
	n, ok := parseCanonicalNumber(fact.NormalizedValue)
	if !ok {
		return reportCell{Text: "—"}
	}
	if trillions {
		scale, ok := unitScale(fact.Unit)
		if !ok {
			return reportCell{Text: "—"}
		}
		n *= scale / 1e12
	}
	return reportCell{Links: []reportLink{{fmt.Sprintf("%.2f", n), pageURL(fact.SourceURL, fact.PageNumber)}}}
}

func (r *AnalysisReport) observationCell(obs analysisObservation, scale float64) reportCell {
	if obs.Value == nil {
		return reportCell{Text: "—"}
	}
	return reportCell{Links: []reportLink{{fmt.Sprintf("%.2f", *obs.Value*scale), r.Sources[obs.Source].URL}}}
}

func basisLabel(basis string) string {
	if basis == "unconsolidated" {
		return "bank only"
	}
	return basis
}

func (r *AnalysisReport) annualTable(brief bool) reportTable {
	table := reportTable{Headers: []string{"FY", "Assets", "Group profit", "Operating cash", "Reported EPS", "ROA", "ROE"}}
	if !brief {
		table.Headers = []string{"FY", "Assets", "Group equity", "Parent equity", "Group profit", "Parent profit", "Reported EPS"}
	}
	for _, row := range r.Annual {
		if brief && row.Period < "2021" {
			continue
		}
		f := row.Facts
		eps := r.observationCell(r.findObservation("Basic EPS on 2025 share basis", "BBCA", row.Period, "consolidated", "IDR/share"), 1)
		var cells []reportCell
		if brief {
			cells = []reportCell{factCell(f["total_assets"], true), factCell(f["net_earnings"], true),
				factCell(f["net_cash_from_operating_activities"], true), eps,
				{Text: metricText(row.Values["return-on-assets"])}, {Text: metricText(row.Values["return-on-equity"])}}
		} else {
			cells = []reportCell{factCell(f["total_assets"], true), factCell(f["shareholders_equity"], true),
				factCell(f["common_shareholders_equity"], true), factCell(f["net_earnings"], true),
				factCell(f["net_income_available_to_common_shareholders"], true), eps}
		}
		table.Rows = append(table.Rows, append(textCells(row.Period), cells...))
	}
	return table
}

func (r *AnalysisReport) interimTable(brief bool) reportTable {
	table := reportTable{Headers: []string{"Period", "Basis", "Assets (IDR T)", "Equity (IDR T)", "YTD profit (IDR T)"}}
	if brief {
		table.Headers = append(table.Headers, "Profit YoY")
	}
	for _, row := range r.Interim {
		if brief && !(strings.HasPrefix(row.Period, "2026-") && (row.Basis == "consolidated" || row.Period == "2026-10")) {
			continue
		}
		position := func(key, supplemental string) reportCell {
			if fact, ok := row.Facts[key]; ok {
				return factCell(fact, true)
			}
			return r.observationCell(r.findObservation(supplemental, "BBCA", row.Period, row.Basis, "IDR million"), 1e-6)
		}
		cells := append(textCells(row.Period, basisLabel(row.Basis)), position("total_assets", "Closing assets"),
			position("shareholders_equity", "Closing equity"), factCell(row.Facts["net_earnings"], true))
		if brief {
			cells = append(cells, reportCell{Text: metricText(row.Values["interim-net-earnings-yoy"])})
		}
		table.Rows = append(table.Rows, cells)
	}
	return table
}

func (r *AnalysisReport) bankingTables() []reportTable {
	bank := reportTable{Title: "Annual operating indicators", Headers: []string{"Measure", "Basis", "Unit", "FY2024", "FY2025"}}
	type series struct{ key, basis, unit string }
	items := []series{
		{"Gross loans and financing", "consolidated", "IDR billion"}, {"Third-party funds", "consolidated", "IDR billion"},
		{"CASA ratio", "consolidated", "%"}, {"Gross NPL", "bank only", "%"}, {"Loan at risk", "consolidated", "%"},
		{"Net interest margin", "bank only", "%"}, {"Cost to income", "bank only", "%"},
		{"Impairment losses", "consolidated", "IDR million"}, {"Loan to deposit ratio", "bank only", "%"},
	}
	for _, item := range items {
		unit, scale := item.unit, 1.0
		switch item.unit {
		case "IDR billion":
			unit, scale = "IDR T", 1e-3
		case "IDR million":
			unit, scale = "IDR T", 1e-6
		}
		row := textCells(item.key, item.basis, unit)
		for _, year := range []string{"2024", "2025"} {
			row = append(row, r.observationCell(r.findObservation(item.key, "BBCA", year, item.basis, item.unit), scale))
		}
		bank.Rows = append(bank.Rows, row)
	}
	risk := reportTable{Title: "Consolidated capital and liquidity (KM1, %)", Headers: []string{"Period", "Capital adequacy", "Liquidity coverage", "Net stable funding"}}
	for _, period := range []string{"2025-09", "2025-12", "2026-03", "2026-06", "2026-09"} {
		if period != "2025-12" && period != "2026-09" {
			continue
		}
		row := textCells(period)
		for _, key := range []string{"Capital adequacy ratio", "Liquidity coverage ratio", "Net stable funding ratio"} {
			row = append(row, r.observationCell(r.findObservation(key+" (KM1)", "BBCA", period, "consolidated", "%"), 1))
		}
		risk.Rows = append(risk.Rows, row)
	}
	return []reportTable{bank, risk}
}

func (r *AnalysisReport) briefSections() []reportSection {
	valuation := reportTable{Headers: []string{"Measure at FY2025 close", "Result"}}
	for _, id := range []string{"price-earnings-ratio", "price-to-book-ratio", "dividend-yield", "margin-of-safety"} {
		valuation.Rows = append(valuation.Rows, textCells(displayKey(id), metricText(r.Results[id])))
	}
	close := r.findObservation("Year-end close", "BBCA", "2025", "market", "IDR/share")
	valuation.Rows = append(valuation.Rows, append(textCells("Year-end share price (IDR/share)"), r.observationCell(close, 1)))
	cases := reportTable{Title: "Capital-constrained dividend value", Headers: []string{"Scenario", "Value (IDR/share)", "Payout target", "CET1 target", "Cost of equity", "Terminal growth"}}
	for _, name := range []string{"conservative", "base", "favorable", "weighted"} {
		metric := r.Results["dividend-value-"+name]
		row := textCells(name, "—")
		if metric.Value != nil {
			row[1].Text = fmt.Sprintf("%.2f", *metric.Value)
		}
		for _, key := range []string{"payout_target", "cet1_target", "cost_of_equity", "terminal_growth"} {
			cell := "—"
			if v, ok := metric.Inputs[key].(float64); ok {
				cell = fmt.Sprintf("%.0f%%", v*100)
			}
			row = append(row, reportCell{Text: cell})
		}
		cases.Rows = append(cases.Rows, row)
	}
	return []reportSection{
		{Paragraphs: []reportCell{{Text: "Audited FY2025 with reviewed interim disclosures through October 2026."}}},
		{ID: "what-changed", Title: "What changed", Paragraphs: []reportCell{
			{Text: "FY2025 profits grew as net interest and fee income increased. Impairment costs rose even as the headline nonperforming-loan ratio improved. Assets grew faster than earnings and returns eased.",
				Links: r.sourceLinks("income", "ratios")},
			{Text: "September 2026 year-to-date profit growth accelerated from the June comparison. The consolidated capital and liquidity ratios were below their FY2025 levels. The October publication covers the bank only.",
				Links: r.sourceLinks("interim", "km1", "oct")},
		}},
		{ID: "annual", Title: "Five-year operating trend", Paragraphs: []reportCell{{Text: "Consolidated amounts in IDR trillion. Basic EPS is in IDR per share on the October 2021 share basis."}}, Tables: []reportTable{r.annualTable(true)}},
		{ID: "interim", Title: "2026 interim update", Paragraphs: []reportCell{{Text: "Year-over-year profit compares matching year-to-date periods within each reporting basis. The October comparison uses the bank-only publications.", Links: r.sourceLinks("bca_monthly_published_financial_report_2025-10-p3", "oct")}}, Tables: []reportTable{r.interimTable(true)}},
		{ID: "risk", Title: "Funding, asset quality and capital", Tables: r.bankingTables()},
		{ID: "valuation", Title: "Valuation at the FY2025 close", Paragraphs: []reportCell{
			{Text: "The retrospective scenarios use five years of common earning power and the CET1 required for projected growth. The weighted value uses conservative, base and favorable weights of 25%, 50% and 25%. Later disclosures supply some FY2025 inputs.",
				Links: r.sourceLinks("balance", "price", "car", "km1")},
			{Text: "Full history, rule dispositions and calculation records.",
				Links: []reportLink{{"Evidence companion", "bbca-fy2025-evidence.md"}}},
		}, Tables: []reportTable{valuation, cases}},
	}
}

func (r *AnalysisReport) sourceLinks(keys ...string) []reportLink {
	var links []reportLink
	for _, key := range keys {
		if src, ok := r.Sources[key]; ok {
			links = append(links, reportLink{src.Title + " · " + src.Where, src.URL})
		}
	}
	return links
}

func (r *AnalysisReport) evidenceSections() ([]reportSection, error) {
	sections := []reportSection{
		{Paragraphs: []reportCell{{Text: "Audited FY2025 with reviewed interim disclosures through October 2026."}, {Text: "Reviewed facts SHA-256 " + r.FactsHash}, {Text: "Catalog SHA-256 " + r.CatalogHash}}},
		{ID: "history", Title: "Annual financial history", Paragraphs: []reportCell{{Text: "Consolidated amounts in IDR trillion. This table puts reported EPS on the October 2021 share basis. EPS calculation records show the share counts on the basis documented in the cited filing."}}, Tables: []reportTable{r.annualTable(false)}},
		{ID: "interim", Title: "Interim statement history", Tables: []reportTable{r.interimTable(false)}},
	}
	if r.BankCatalogHash != "" {
		sections[0].Paragraphs = append(sections[0].Paragraphs, reportCell{Text: "Bank overlay SHA-256 " + r.BankCatalogHash})
	}
	notes := reportTable{Title: "Periods and comparability", Headers: []string{"Period", "Basis", "Note"}}
	for _, row := range append(append([]analysisPeriod{}, r.Annual...), r.Interim...) {
		if row.Note != "" {
			notes.Rows = append(notes.Rows, textCells(row.Period, basisLabel(row.Basis), row.Note))
		}
	}
	sections[2].Tables = append(sections[2].Tables, notes)
	ruleNumbers := make(map[string]int)
	for _, rule := range r.Rules {
		ruleNumbers[rule.Rule.ID] = rule.Number
	}
	coverage := reportTable{Headers: []string{"Source file", "Period coverage", "Status", "Catalog rules"}}
	for _, doc := range r.Coverage.Documents {
		periods := strings.Join(doc.Periods, ", ")
		if periods == "" {
			periods = doc.Period
		}
		status := fmt.Sprintf("%s. %d approved, %d pending, %d pending matches to approved values.", doc.Role, doc.Approved, doc.Pending, doc.Corroborated)
		if doc.ReviewedInputs > 0 {
			status += fmt.Sprintf(" %d reviewed statement inputs.", doc.ReviewedInputs)
		}
		var numbers []int
		for _, id := range doc.Rules {
			if number, ok := ruleNumbers[id]; ok {
				numbers = append(numbers, number)
			}
		}
		sort.Ints(numbers)
		ruleRefs := make([]string, 0, len(numbers))
		for _, number := range numbers {
			ruleRefs = append(ruleRefs, strconv.Itoa(number))
		}
		row := []reportCell{{Links: []reportLink{{path.Base(doc.Path), doc.URL}}}, {Text: periods}, {Text: status}, {Text: strings.Join(ruleRefs, ", ")}}
		coverage.Rows = append(coverage.Rows, row)
	}
	sections = append(sections, reportSection{ID: "coverage", Title: "Local source coverage", Tables: []reportTable{coverage},
		Paragraphs: []reportCell{{Text: "Rule mappings include the measures exposed by each source and the catalog assessments citing its disclosures. Rule numbers follow the catalog order shown in Rule assessments. External sources and manually verified cells appear in the source register."}}})
	records, err := r.calculationSections()
	if err != nil {
		return nil, err
	}
	calcsByRule := make(map[string][]reportLink)
	for _, item := range records {
		if item.Rule != "" {
			calcsByRule[item.Rule] = append(calcsByRule[item.Rule], reportLink{item.Label, "#" + item.Section.ID})
		}
	}
	assessments := reportTable{Headers: []string{"Rule", "Disposition", "Evidence and gaps", "Calculations"}}
	for _, rule := range r.Rules {
		finding := rule.Finding
		if rule.Rule.Type == "formula" && rule.Rule.Formula != "" {
			finding += " Formula " + rule.Rule.Formula + "."
		}
		if rule.Gap != "" {
			finding += " " + rule.Gap
		}
		if rule.Metric != nil && rule.Metric.Value == nil && rule.Metric.Reason != "" {
			finding += " " + rule.Metric.Reason
		}
		if r.BankCatalogHash != "" && rule.Metric != nil && rule.Metric.EntityID == bankNPLCoverageRule && rule.Metric.Reason != "" {
			finding += " " + rule.Metric.Reason
		}
		assessments.Rows = append(assessments.Rows, []reportCell{{Text: fmt.Sprintf("%d · %s", rule.Number, rule.Rule.ID)}, {Text: displayKey(rule.Status)},
			{Text: finding, Links: r.sourceLinks(rule.Sources...)}, {Links: calcsByRule[rule.Rule.ID]}})
	}
	sections = append(sections, reportSection{ID: "rules", Title: "Rule assessments", Tables: []reportTable{assessments}})
	if r.BankCatalogHash != "" {
		bankMetrics := reportTable{Headers: []string{"Measure", "Period", "Scope", "Result", "Status", "Input gaps or limits", "Input sources"}}
		for _, metric := range r.BankMetrics {
			status := strings.ReplaceAll(displayKey(metric.Status), "_", " ")
			row := textCells(displayKey(metric.EntityID), metric.Period,
				basisLabel(metric.Basis), metricText(metric), status, metric.Reason)
			row = append(row, reportCell{Links: r.sourceLinks(bankMetricSourceIDs(metric)...)})
			bankMetrics.Rows = append(bankMetrics.Rows, row)
		}
		sections = append(sections, reportSection{ID: "bank-metrics", Title: "Bank-specific calculations and gaps",
			Paragraphs: []reportCell{{Text: "Each ratio uses reviewed inputs on matching reporting dates, flow periods, units, and scope. Interim ratios are period-to-date and are not annualized. Missing or conflicting inputs remain visible as gaps."}},
			Tables:     []reportTable{bankMetrics}})
	}
	if r.BankScenarios != nil {
		sections = append(sections, r.yearlyBankScenarios())
	}
	sections = append(sections, r.periodGapSection())
	sections = append(sections, reportSection{ID: "research-gaps", Title: "Remaining research gaps", Paragraphs: []reportCell{
		{Text: "BCA’s audited equity statement confirms the FY1998 year-end deficit carried into FY1999. The FY1999 recapitalization restored positive closing equity, but average equity was negative, making conventional ROE unsuitable. The FY1998 cash-flow, tax and asset-return calculations still need the full statements and FY1997 opening assets. A ten-year capex ratio before FY2008 also needs contemporaneous fixed-asset purchases and earnings.",
			Links: r.sourceLinks("additional-fy2002-p2", "archive00-equity", "archive00-balance")},
		{Text: "The audited financing cash flows and BCA dividend records support zero cash dividends paid in calendar 2000. The fiscal FY2000 payout still needs the 31 May 2001 shareholder resolution or equivalent evidence. A full-calendar 2000 shareholder return is unavailable because BBCA listed during the year. FY2026 annual results await publication.",
			Links: r.sourceLinks("archive00-cash", "dividend-record", "bca_ar_2002-p171", "split", "oct")},
		{Text: "Owner earnings and replacement value depend on estimates of maintenance reinvestment and replacement costs. Look-through adjustments require material unrecognized investee earnings and distribution taxes. Portfolio rules need investor inputs. Market-strategy rules need an investable, point-in-time market dataset including delisted firms and trading costs.",
			Links: r.sourceLinks("cash", "leases", "subs", "bank")},
	}})
	sections = append(sections, reportSection{ID: "calculations", Title: "Calculation records", Paragraphs: []reportCell{{Text: "Values use the cited input units. Linked inputs refer to another calculation in this companion. The rule assessments give the catalog formulas. The valuation assumptions and funding constraints are described in the brief and carried in the scenario inputs."}}})
	for _, record := range records {
		sections = append(sections, record.Section)
	}

	observations := reportTable{Headers: []string{"Measure", "Period", "Basis", "Value and unit", "Evidence"}}
	for _, obs := range r.Observations {
		switch obs.Key {
		case "Basic EPS on 2025 share basis", "Closing assets", "Closing equity":
			// These values already have their filing links in the period tables.
			continue
		case "Basic EPS as published":
			adjusted := r.findObservation("Basic EPS on 2025 share basis", obs.Subject, obs.Period, obs.Basis, obs.Unit)
			if adjusted.Value != nil && *adjusted.Value == *obs.Value && adjusted.Source == obs.Source {
				continue
			}
		}
		basis := obs.Basis
		if r.BankCatalogHash != "" {
			basis = basisLabel(basis)
		}
		row := textCells(obs.Subject+" · "+obs.Key, obs.Period, basis, fmt.Sprintf("%s %s", strconv.FormatFloat(*obs.Value, 'f', -1, 64), obs.Unit))
		observations.Rows = append(observations.Rows, append(row, reportCell{Links: r.sourceLinks(obs.Source, obs.Corroboration)}))
	}
	sections = append(sections, reportSection{ID: "observations", Title: "Reviewed disclosures and observations", Tables: []reportTable{observations}})
	sections = append(sections, r.candidateReviewSection())

	register := reportTable{Headers: []string{"Reference", "Locator"}}
	for _, id := range sortedKeys(r.Sources) {
		src := r.Sources[id]
		register.Rows = append(register.Rows, []reportCell{{Links: []reportLink{{src.Title, src.URL}}}, {Text: src.Where}})
	}
	sections = append(sections, reportSection{ID: "sources", Title: "Source register", Tables: []reportTable{register}})
	return sections, nil
}

func bankMetricSourceIDs(metric Metric) []string {
	seen := make(map[string]bool)
	var result []string
	add := func(source string) {
		if source != "" && !seen[source] {
			seen[source] = true
			result = append(result, source)
		}
	}
	for _, input := range metric.Inputs {
		switch value := input.(type) {
		case analysisObservation:
			add(value.Source)
			add(value.Corroboration)
		case *analysisObservation:
			if value != nil {
				add(value.Source)
				add(value.Corroboration)
			}
		case []bankInputConflict:
			for _, conflict := range value {
				for _, source := range conflict.Sources {
					add(source)
				}
			}
		}
	}
	sort.Strings(result)
	return result
}

func (r *AnalysisReport) periodGapSection() reportSection {
	type gap struct{ id, basis, reason string }
	periods := make(map[gap][]string)
	for _, row := range append(append([]analysisPeriod{}, r.Annual...), r.Interim...) {
		for id, metric := range row.Values {
			headline := r.Results[id]
			if metric.Value != nil || row.Basis == "consolidated" && headline.Period == row.Period && headline.Value != nil {
				continue
			}
			reason := metric.Reason
			switch metric.Status {
			case "missing_inputs", "conflict", "not_calculable":
				if required, ok := metric.Inputs["missing"].([]string); ok {
					reason = "Missing inputs " + strings.Join(required, ", ")
					if id == "capital-expenditure-to-net-earnings" {
						reason = compactWindowGap(required)
					}
				}
				if row.Basis == "unconsolidated" && strings.HasSuffix(row.Period, "-10") {
					switch id {
					case "net-change-in-cash":
						reason = "The monthly bank-only publication omits the cash-flow statement."
					case "earnings-per-share":
						reason = "The monthly publication omits the weighted-average shares for the January–October period."
					case "net-profit-margin":
						reason = "Needs a reviewed operating-income total from the same bank-only presentation."
					}
				}
			case "requires_external_data":
				if len(row.Period) != 4 || (id != "price-earnings-ratio" && id != "price-to-book-ratio" &&
					id != "dividend-yield" && id != "real-return-after-inflation") {
					continue
				}
				reason = map[string]string{
					"price-earnings-ratio":        "Requires a reviewed year-end price and positive basic EPS on the same split basis.",
					"price-to-book-ratio":         "Requires a reviewed year-end price and positive same-basis book value per share.",
					"dividend-yield":              "Requires reviewed calendar-year cash payments and a year-end price on the same share basis.",
					"real-return-after-inflation": "Requires a reviewed nominal holding-period return and CPI growth for that period.",
				}[id]
			default:
				continue
			}
			key := gap{id, row.Basis, reason}
			periods[key] = append(periods[key], row.Period)
		}
	}
	keys := make([]gap, 0, len(periods))
	for key := range periods {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return (keys[i].id + "|" + keys[i].basis + "|" + keys[i].reason) < (keys[j].id + "|" + keys[j].basis + "|" + keys[j].reason)
	})
	table := reportTable{Headers: []string{"Calculation", "Basis", "Periods", "Gap"}}
	for _, key := range keys {
		table.Rows = append(table.Rows, textCells(displayKey(key.id), basisLabel(key.basis), strings.Join(periods[key], ", "), key.reason))
	}
	return reportSection{ID: "period-gaps", Title: "Calculation gaps by period", Tables: []reportTable{table}}
}

func compactWindowGap(required []string) string {
	var series []string
	for _, key := range []string{"capital_expenditures", "net_earnings"} {
		var years []string
		for _, item := range required {
			if year, ok := strings.CutPrefix(item, key+"@"); ok {
				years = append(years, year)
			}
		}
		if len(years) > 0 {
			series = append(series, strings.ReplaceAll(key, "_", " ")+" for "+strings.Join(years, ", "))
		}
	}
	return "Missing " + strings.Join(series, ". Missing ")
}

func (r *AnalysisReport) candidateReviewSection() reportSection {
	table := reportTable{Headers: []string{"Measure and scope", "Value", "Source candidates"}}
	for _, group := range r.Coverage.Candidates {
		if len(group.Values) == 1 && len(group.Values[0].Sources) == 1 {
			continue
		}
		scope := strings.Join([]string{group.Measure, group.Period, basisLabel(group.Basis), group.Audit, group.Kind}, " · ")
		if group.Start != "" {
			scope += " · " + group.Start + " to " + group.End
		}
		for _, value := range group.Values {
			row := textCells(scope, strings.TrimSpace(value.Value+" "+group.Currency+" "+group.Unit))
			var refs []reportLink
			for _, src := range value.Sources {
				refs = append(refs, reportLink{path.Base(src.Document) + " · " + src.Locator + " · " + src.Status, pageURL(src.URL, src.Page)})
			}
			table.Rows = append(table.Rows, append(row, reportCell{Links: refs}))
		}
	}
	return reportSection{ID: "candidate-review", Title: "Corroborating and divergent candidates",
		Paragraphs: []reportCell{{Text: "An identical value within a reporting scope appears once with every locator. Distinct values remain visible. A difference can reflect rounding, classification, restatement or extraction and requires review before use."}},
		Tables:     []reportTable{table}}
}

type calculationRecord struct {
	Section reportSection
	Rule    string
	Label   string
}

type calculationKey struct{ id, period, basis string }

func (k calculationKey) anchor() string {
	return "calc-" + k.id + "-" + k.period + "-" + strings.ReplaceAll(strings.ToLower(k.basis), " ", "-")
}

func (r *AnalysisReport) calculationSections() ([]calculationRecord, error) {
	all := make(map[calculationKey]Metric)
	for id, metric := range r.Results {
		if metric.Value != nil {
			basis := metric.Basis
			if basis == "" {
				basis = "consolidated"
			}
			if metric.RelatedRule != "" {
				id = metric.EntityID
			}
			all[calculationKey{id, metric.Period, basis}] = metric
		}
	}
	for _, metric := range r.BankMetrics {
		if metric.Value == nil {
			continue
		}
		basis := metric.Basis
		if basis == "" {
			basis = "consolidated"
		}
		all[calculationKey{metric.EntityID, metric.Period, basis}] = metric
	}
	for _, row := range append(append([]analysisPeriod{}, r.Annual...), r.Interim...) {
		for id, metric := range row.Values {
			key := calculationKey{id, row.Period, row.Basis}
			if _, exists := all[key]; !exists && metric.Value != nil {
				all[key] = metric
			}
		}
	}
	keys := make([]calculationKey, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].anchor() < keys[j].anchor() })
	var records []calculationRecord
	for _, key := range keys {
		metric := all[key]
		table := reportTable{Headers: []string{"Input", "Reviewed value or reference"}}
		data, err := json.Marshal(metric.Inputs)
		if err != nil {
			return nil, err
		}
		var values map[string]any
		if err := json.Unmarshal(data, &values); err != nil {
			return nil, err
		}
		for _, name := range sortedKeys(values) {
			r.appendCalculationInput(&table, displayKey(name), values[name], all, key.basis)
		}
		calculationSummary := metricText(metric)
		if r.BankCatalogHash != "" && metric.EntityID == bankNPLCoverageRule && metric.Status == "proxy_calculated" {
			calculationSummary = "Proxy · " + calculationSummary + ". " + metric.Reason
		}
		section := reportSection{ID: key.anchor(), Title: displayKey(key.id) + " · " + key.period + " · " + basisLabel(key.basis),
			Paragraphs: []reportCell{{Text: calculationSummary}}}
		if len(table.Rows) > 0 {
			section.Tables = []reportTable{table}
		}
		rule, label := key.id, key.period
		if metric.RelatedRule != "" {
			rule, label = metric.RelatedRule, key.period+" · "+basisLabel(key.basis)
		}
		records = append(records, calculationRecord{section, rule, label})
	}
	return records, nil
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r *AnalysisReport) appendCalculationInput(table *reportTable, label string, item any, records map[calculationKey]Metric, basis string) {
	add := func(cell reportCell) { table.Rows = append(table.Rows, []reportCell{{Text: label}, cell}) }
	switch v := item.(type) {
	case map[string]any:
		if id, ok := v["entity_id"].(string); ok {
			period, _ := v["period"].(string)
			key := calculationKey{id, period, basis}
			if _, exists := records[key]; exists {
				add(reportCell{Links: []reportLink{{displayKey(id) + " · " + period, "#" + key.anchor()}}})
				return
			}
		}
		value, hasValue := v["value"]
		if raw, ok := v["normalized_value"]; ok {
			value, hasValue = raw, true
		}
		if hasValue {
			unit, _ := v["unit"].(string)
			currency, _ := v["currency"].(string)
			cell := reportCell{Text: strings.TrimSpace(fmt.Sprint(value) + " " + currency + " " + unit)}
			source, _ := v["source"].(string)
			corroboration, _ := v["corroboration"].(string)
			cell.Links = r.sourceLinks(source, corroboration)
			if link, ok := calculationSource(v); ok {
				cell.Links = append(cell.Links, link)
			}
			if facts, ok := v["source_facts"].([]any); ok {
				for _, fact := range facts {
					if row, ok := fact.(map[string]any); ok {
						if link, ok := calculationSource(row); ok {
							cell.Links = append(cell.Links, link)
						}
					}
				}
			}
			add(cell)
			return
		}
		for _, key := range sortedKeys(v) {
			r.appendCalculationInput(table, label+" / "+displayKey(key), v[key], records, basis)
		}
	case []any:
		for i, value := range v {
			r.appendCalculationInput(table, fmt.Sprintf("%s / %d", label, i+1), value, records, basis)
		}
	case float64:
		add(reportCell{Text: fmt.Sprintf("%.12g", v)})
	case string:
		add(reportCell{Text: v})
	}
}

func calculationSource(v map[string]any) (reportLink, bool) {
	url, ok := v["source_url"].(string)
	if !ok || url == "" {
		return reportLink{}, false
	}
	page, _ := v["page_number"].(float64)
	doc, _ := v["document_path"].(string)
	locator, _ := v["source_locator"].(string)
	return reportLink{strings.TrimSpace(path.Base(doc) + " " + locator), pageURL(url, int(page))}, true
}
