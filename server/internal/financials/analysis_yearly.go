package financials

import (
	"fmt"
	"math"
	"strings"

	"github.com/Chandra179/alcazar/server/internal/report"
)

type analysisYearStory struct {
	Title   string   `yaml:"title"`
	Bank    string   `yaml:"bank"`
	Impact  string   `yaml:"impact"`
	Sources []string `yaml:"sources"`
}

type yearlyValue struct {
	value float64
	unit  string
	basis string
	links []reportLink
	valid bool
}

type yearlyMeasure struct {
	group  string
	label  string
	unit   string
	get    func(analysisPeriod) yearlyValue
	getFor func(analysisPeriod, string) yearlyValue
}

func (r *AnalysisReport) yearlySections(stories map[string]analysisYearStory, storySources map[string]analysisSource) ([]reportSection, error) {
	if stories == nil {
		return nil, nil
	}
	const interimYear = "2026"
	sources := make(map[string]analysisSource, len(r.Sources)+len(storySources))
	for key, source := range r.Sources {
		sources[key] = source
	}
	for key, source := range storySources {
		if _, exists := sources[key]; exists || source.Title == "" || source.Where == "" || !strings.HasPrefix(source.URL, "https://") {
			return nil, fmt.Errorf("invalid or duplicate yearly story source %q", key)
		}
		sources[key] = source
	}
	if len(stories) != len(r.Annual)+1 {
		return nil, fmt.Errorf("yearly stories must cover every annual period and the %s interim update", interimYear)
	}
	for year, story := range stories {
		if strings.TrimSpace(story.Title) == "" || strings.TrimSpace(story.Bank) == "" ||
			strings.TrimSpace(story.Impact) == "" || len(story.Sources) < 1 || len(story.Sources) > 2 {
			return nil, fmt.Errorf("incomplete yearly story for %s", year)
		}
		for _, key := range story.Sources {
			if _, ok := sources[key]; !ok {
				return nil, fmt.Errorf("yearly story %s references unknown source %q", year, key)
			}
		}
	}
	interim, ok := stories[interimYear]
	if !ok {
		return nil, fmt.Errorf("missing yearly story for %s", interimYear)
	}

	if len(r.Annual) == 0 {
		return nil, fmt.Errorf("yearly report requires at least one annual period")
	}
	cover := reportSection{ID: "yearly-cover", Kind: "cover", Paragraphs: []reportCell{{
		Text: "A year-by-year comparison of BCA’s business, financial position and shareholder outcomes, from the first available annual record through FY2025, with a 2026 update.",
	}}}
	sections := []reportSection{cover, r.yearlyInterim(interim, sources)}
	if r.profile().Industry != "corporate" {
		if r.BankCatalogHash != "" {
			sections = append(sections, r.yearlyBankRisk())
		}
		if r.BankScenarios != nil {
			sections = append(sections, r.yearlyBankScenarios())
		}
	} else if corp := r.yearlyCorporate(); corp != nil {
		sections = append(sections, *corp)
	}
	for i := len(r.Annual) - 1; i >= 0; i-- {
		row := r.Annual[i]
		story, ok := stories[row.Period]
		if !ok {
			return nil, fmt.Errorf("missing yearly story for %s", row.Period)
		}
		var prior Fact
		if i > 0 {
			prior = r.Annual[i-1].Facts["net_earnings"]
		}
		close := r.marketShareObservation("Year-end close", row.Period)
		table := r.yearlyComparisonTable(i)
		sections = append(sections, reportSection{
			ID: "year-" + row.Period, Kind: "annual", PeriodLabel: row.Period,
			Title: row.Period + " — " + story.Title,
			Paragraphs: []reportCell{
				{Label: "At the bank", Text: strings.TrimSpace(story.Bank + qualitativeProfitSummary(row.Facts["net_earnings"], prior))},
				{Label: "For shareholders", Text: r.yearlyStockSummary(row, close)},
				{Label: "Why it mattered", Text: story.Impact},
				{Label: "Sources", Links: yearlySourceLinks(sources, story.Sources, close.Source, false)},
			},
			Tables: []reportTable{table},
		})
	}
	sections = append(sections, yearlyGlossary())
	if charts := report.YearlyChartSections(sections); len(charts) > 0 {
		updated := make([]reportSection, 0, len(sections)+len(charts))
		updated = append(updated, sections[0])
		updated = append(updated, charts...)
		updated = append(updated, sections[1:]...)
		sections = updated
	}
	sections[0].Tables = []reportTable{report.YearlyContents(sections[1:])}
	return report.PrepareYearly(sections), nil
}

func qualitativeProfitSummary(current, prior Fact) string {
	profit, ok := annualProfitInTrillions(current)
	if !ok {
		return ""
	}
	if profit < 0 {
		return " The group reported a loss for the year."
	}
	previous, ok := annualProfitInTrillions(prior)
	if !ok || prior.Period != previousBalancePeriod(current.Period) || current.Period == bbcaListingYear || prior.Period == bbcaListingYear || previous == 0 {
		return " The group remained profitable."
	}
	if previous < 0 && profit > 0 {
		return " The group returned to profit after a loss the year before."
	}
	change := (profit/previous - 1) * 100
	if math.Abs(change) < 0.05 {
		return " Group profit was roughly unchanged."
	}
	if change < 0 {
		return " Group profit declined from the year before."
	}
	return " Group profit increased from the year before."
}

func annualProfitInTrillions(fact Fact) (float64, bool) {
	value, ok := parseCanonicalNumber(fact.NormalizedValue)
	scale, supported := unitScale(fact.Unit)
	return value * scale / 1e12, ok && supported && isMoneyScale(fact.Unit) && fact.Currency == "IDR" &&
		fact.ReportingBasis == "consolidated" && fact.AuditedStatus == "audited" && fact.PeriodType == "annual" && samePeriodFlow(fact)
}

func (r *AnalysisReport) yearlyStockSummary(row analysisPeriod, close analysisObservation) string {
	if row.Period < bbcaListingYear {
		return "Before listing; the shares were not yet traded on the exchange."
	}
	if row.Period == bbcaListingYear {
		return "BCA listed during the year, so there is no full-calendar shareholder return. The listing-year closing price is shown in the table."
	}
	open := r.marketShareObservation("Year-end close", previousBalancePeriod(row.Period))
	if open.Value == nil || *open.Value <= 0 || close.Value == nil || *close.Value <= 0 {
		return "A comparable full-year share-price return could not be verified for this year."
	}
	price := 100 * (*close.Value - *open.Value) / *open.Value
	total := row.Values["nominal-shareholder-return"].Value
	if total == nil {
		if price < 0 {
			return "The share price fell; the dividend-inclusive return is unavailable in the reviewed records."
		}
		return "The share price rose; the dividend-inclusive return is unavailable in the reviewed records."
	}
	switch {
	case price < 0 && *total < 0:
		return "The share price fell. Cash dividends softened the decline, but the annual return remained negative."
	case price < 0 && *total >= 0:
		return "The share price fell, but cash dividends more than offset the decline for a full-year holder."
	case price > 0 && *total > price:
		return "The share price rose, and cash dividends added to the full-year return."
	case price > 0 && *total < price:
		return "The share price rose, while the dividend-inclusive return was lower."
	default:
		return "The share price and dividend-inclusive return were broadly aligned."
	}
}

func (r *AnalysisReport) yearlyComparisonTable(index int) reportTable {
	row := r.Annual[index]
	var prior analysisPeriod
	if index > 0 {
		prior = r.Annual[index-1]
	}
	period := row.Period
	measures := r.yearlyMeasures()
	priorLabel := prior.Period
	if priorLabel == "" {
		priorLabel = "—"
	}
	table := reportTable{Headers: []string{"Measure", priorLabel, period, "Change"},
		NumericColumns: []int{1, 2, 3}, CurrentColumn: 2}
	groupRows := make(map[string][][]reportCell)
	for _, measure := range measures {
		current := yearlyValue{}
		if measure.get != nil {
			current = measure.get(row)
		} else if measure.getFor != nil {
			current = measure.getFor(row, "")
		}
		if !current.valid {
			continue
		}
		previous := yearlyValue{}
		if index > 0 {
			if measure.get != nil {
				previous = measure.get(prior)
			} else if measure.getFor != nil {
				previous = measure.getFor(prior, current.basis)
			}
		}
		label := measure.label
		if current.basis == "bank only" || current.basis == "unconsolidated" {
			label += " (" + current.basis + ")"
		} else if current.basis == "consolidated" && measure.getFor != nil {
			label += " (group)"
		}
		change := yearlyChange(previous, current, measure.unit, period)
		groupRows[measure.group] = append(groupRows[measure.group], []reportCell{
			{Text: label},
			yearlyCitationCell(previous, measure.unit),
			yearlyCitationCell(current, measure.unit),
			{Text: change},
		})
	}
	if period == bbcaListingYear {
		close := r.marketShareObservation("Year-end close", period)
		if close.Value != nil && *close.Value > 0 {
			price := yearlyValue{value: *close.Value, unit: "price", basis: "market", links: r.observationLinks(close), valid: true}
			groupRows["Shareholder outcome"] = append(groupRows["Shareholder outcome"], []reportCell{
				{Text: "Year-end share price (listing year)"}, {Text: "—"},
				yearlyCitationCell(price, "price"), {Text: "listing year"},
			})
		}
	}

	for _, group := range []string{"Business growth", "Earnings", "Credit risk", "Capital and liquidity", "Shareholder outcome"} {
		rows := groupRows[group]
		if len(rows) == 0 {
			continue
		}
		table.Rows = append(table.Rows, []reportCell{{Text: group}, {}, {}, {}})
		table.Rows = append(table.Rows, rows...)
	}
	return table
}

func (r *AnalysisReport) yearlyMeasures() []yearlyMeasure {
	return []yearlyMeasure{
		{group: "Business growth", label: "Loans", unit: "money", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Gross loans and financing", row.Period, basis, "IDR billion", "money", 1e-3)
		}},
		{group: "Business growth", label: "Customer deposits", unit: "money", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Third-party funds", row.Period, basis, "IDR billion", "money", 1e-3)
		}},
		{group: "Business growth", label: "Low-cost deposit mix (CASA)", unit: "percent", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("CASA ratio", row.Period, basis, "%", "percent", 1)
		}},
		{group: "Earnings", label: "Group profit", unit: "money", get: func(row analysisPeriod) yearlyValue {
			return yearlyFact(row.Facts["net_earnings"], "money", "consolidated")
		}},
		{group: "Earnings", label: "Earnings per share", unit: "eps", get: func(row analysisPeriod) yearlyValue {
			if metric := row.Values["earnings-per-share"]; metric.Value != nil {
				return r.yearlyMetric(metric, "eps")
			}
			return yearlyValue{}
		}},
		{group: "Earnings", label: "Return on assets", unit: "percent", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["return-on-assets"], "percent")
		}},
		{group: "Earnings", label: "Return on equity", unit: "percent", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["return-on-equity"], "percent")
		}},
		{group: "Earnings", label: "Net interest margin", unit: "percent", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Net interest margin", row.Period, basis, "%", "percent", 1)
		}},
		{group: "Earnings", label: "Operating cash flow", unit: "money", get: func(row analysisPeriod) yearlyValue {
			return yearlyFact(row.Facts["net_cash_from_operating_activities"], "money", "consolidated")
		}},
		{group: "Earnings", label: "Capital spending / earnings (10-year)", unit: "percent", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["capital-expenditure-to-net-earnings"], "percent")
		}},
		{group: "Credit risk", label: "Gross non-performing loans", unit: "percent", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Gross NPL", row.Period, basis, "%", "percent", 1)
		}},
		{group: "Credit risk", label: "Loans at risk", unit: "percent", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Loan at risk", row.Period, basis, "%", "percent", 1)
		}},
		{group: "Credit risk", label: "Net asset impairment", unit: "money", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Impairment losses", row.Period, basis, "IDR million", "money", 1e-6)
		}},
		{group: "Capital and liquidity", label: "Capital adequacy ratio", unit: "percent", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Capital adequacy ratio", row.Period, basis, "%", "percent", 1)
		}},
		{group: "Capital and liquidity", label: "30-day liquidity coverage", unit: "percent", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Liquidity coverage ratio", row.Period, basis, "%", "percent", 1)
		}},
		{group: "Capital and liquidity", label: "Stable funding ratio", unit: "percent", getFor: func(row analysisPeriod, basis string) yearlyValue {
			return r.yearlyObservation("Net stable funding ratio", row.Period, basis, "%", "percent", 1)
		}},
		{group: "Shareholder outcome", label: "Share-price change", unit: "return", get: func(row analysisPeriod) yearlyValue {
			close := r.marketShareObservation("Year-end close", row.Period)
			open := r.marketShareObservation("Year-end close", previousBalancePeriod(row.Period))
			if row.Period == bbcaListingYear || open.Value == nil || *open.Value <= 0 || close.Value == nil || *close.Value <= 0 {
				return yearlyValue{}
			}
			return yearlyValue{value: (*close.Value / *open.Value - 1) * 100, unit: "return", basis: "market", links: r.observationLinks(close, open), valid: true}
		}},
		{group: "Shareholder outcome", label: "Return including cash dividends", unit: "return", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["nominal-shareholder-return"], "return")
		}},
		{group: "Shareholder outcome", label: "Price / earnings", unit: "multiple", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["price-earnings-ratio"], "multiple")
		}},
		{group: "Shareholder outcome", label: "Price / book value", unit: "multiple", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["price-to-book-ratio"], "multiple")
		}},
		{group: "Shareholder outcome", label: "Dividend yield", unit: "percent", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["dividend-yield"], "percent")
		}},
		{group: "Shareholder outcome", label: "Dividend payout", unit: "percent", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["fiscal-dividend-payout"], "percent")
		}},
		{group: "Shareholder outcome", label: "Return after inflation", unit: "return", get: func(row analysisPeriod) yearlyValue {
			return r.yearlyMetric(row.Values["real-return-after-inflation"], "return")
		}},
	}
}

func (r *AnalysisReport) yearlyObservation(key, period, basis, unit, display string, scale float64) yearlyValue {
	ticker := r.companyTicker()
	var selected analysisObservation
	if basis != "" {
		selected = r.findObservation(key, ticker, period, basis, unit)
	} else {
		for _, preferred := range []string{"consolidated", "bank only", "unconsolidated"} {
			selected = r.findObservation(key, ticker, period, preferred, unit)
			if selected.Value != nil {
				break
			}
		}
	}
	if selected.Value == nil {
		return yearlyValue{}
	}
	return yearlyValue{value: *selected.Value * scale, unit: display, basis: selected.Basis,
		links: r.observationLinks(selected), valid: true}
}

func yearlyFact(fact Fact, display, basis string) yearlyValue {
	value, ok := parseCanonicalNumber(fact.NormalizedValue)
	scale, scaleOK := unitScale(fact.Unit)
	if !ok || !scaleOK || fact.Currency != "IDR" || !isMoneyScale(fact.Unit) || fact.ReportingBasis != basis || !samePeriodFlow(fact) {
		return yearlyValue{}
	}
	value = value * scale / 1e12
	links := []reportLink{}
	if fact.SourceURL != "" {
		links = append(links, reportLink{Text: "Source", URL: pageURL(fact.SourceURL, fact.PageNumber)})
	}
	return yearlyValue{value: value, unit: display, basis: basis, links: links, valid: true}
}

func (r *AnalysisReport) yearlyMetric(metric Metric, display string) yearlyValue {
	if metric.Value == nil || metric.Status != "calculated" {
		return yearlyValue{}
	}
	return yearlyValue{value: *metric.Value, unit: display, basis: metric.Basis, links: r.metricInputLinks(metric), valid: true}
}

func yearlyCitationCell(value yearlyValue, unit string) reportCell {
	return reportCell{Text: displayYearlyValue(value, unit), Links: value.links}
}

func (r *AnalysisReport) observationLinks(observations ...analysisObservation) []reportLink {
	var links []reportLink
	seen := make(map[string]bool)
	for _, observation := range observations {
		if observation.Source == "" {
			continue
		}
		source, ok := r.Sources[observation.Source]
		if !ok || source.URL == "" || seen[source.URL] {
			continue
		}
		seen[source.URL] = true
		links = append(links, reportLink{Text: source.Title, URL: source.URL})
	}
	return links
}

func (r *AnalysisReport) metricInputLinks(metric Metric) []reportLink {
	links := []reportLink{}
	seen := make(map[string]bool)
	add := func(link reportLink) {
		if link.URL != "" && !seen[link.URL] {
			seen[link.URL] = true
			links = append(links, link)
		}
	}
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case Fact:
			add(reportLink{Text: "Source", URL: pageURL(item.SourceURL, item.PageNumber)})
		case analysisObservation:
			for _, link := range r.observationLinks(item) {
				add(link)
			}
		case map[string]any:
			if sourceURL, ok := item["source_url"].(string); ok {
				page := 0
				switch value := item["page_number"].(type) {
				case float64:
					page = int(value)
				case int:
					page = value
				}
				add(reportLink{Text: "Source", URL: pageURL(sourceURL, page)})
			}
			for _, nested := range item {
				visit(nested)
			}
		case []any:
			for _, nested := range item {
				visit(nested)
			}
		case []map[string]any:
			for _, nested := range item {
				visit(nested)
			}
		case []Fact:
			for _, nested := range item {
				visit(nested)
			}
		case []analysisObservation:
			for _, nested := range item {
				visit(nested)
			}
		}
	}
	for _, value := range metric.Inputs {
		visit(value)
	}
	return links
}

func yearlyChange(prior, current yearlyValue, unit, period string) string {
	if period == bbcaListingYear && current.unit == "price" {
		return "listing year"
	}
	if !prior.valid {
		return "level only"
	}
	switch unit {
	case "money":
		if prior.value < 0 && current.value >= 0 {
			return "negative to positive"
		}
		if prior.value >= 0 && current.value < 0 {
			return "positive to negative"
		}
		if prior.value <= 0 {
			if prior.value < 0 && math.Abs(current.value) < math.Abs(prior.value) {
				return "negative amount narrowed"
			}
			if prior.value < 0 {
				return "negative amount widened"
			}
			return "—"
		}
		return fmt.Sprintf("%+.1f%%", (current.value/prior.value-1)*100)
	case "percent":
		return fmt.Sprintf("%+.2f pp", current.value-prior.value)
	case "eps":
		return fmt.Sprintf("%+.0f Rp", current.value-prior.value)
	case "multiple":
		return fmt.Sprintf("%+.2f×", current.value-prior.value)
	case "return":
		return fmt.Sprintf("%+.2f pp", current.value-prior.value)
	default:
		return "—"
	}
}

func displayYearlyValue(value yearlyValue, unit string) string {
	if !value.valid {
		return "—"
	}
	switch unit {
	case "money":
		return fmt.Sprintf("%sT", commaNumber(value.value, 2))
	case "percent":
		return fmt.Sprintf("%.2f%%", value.value)
	case "eps":
		return fmt.Sprintf("%.0f Rp", value.value)
	case "multiple":
		return fmt.Sprintf("%.2f×", value.value)
	case "return":
		return fmt.Sprintf("%+.2f%%", value.value)
	case "price":
		return fmt.Sprintf("%.3f Rp", value.value)
	default:
		return fmt.Sprintf("%.2f", value.value)
	}
}

func commaNumber(value float64, decimals int) string {
	text := fmt.Sprintf("%.*f", decimals, value)
	integer, fraction, hasFraction := strings.Cut(text, ".")
	sign := ""
	if strings.HasPrefix(integer, "-") {
		sign, integer = "-", strings.TrimPrefix(integer, "-")
	}
	var grouped strings.Builder
	grouped.WriteString(sign)
	for index, digit := range integer {
		if index > 0 && (len(integer)-index)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	if hasFraction {
		grouped.WriteByte('.')
		grouped.WriteString(fraction)
	}
	return grouped.String()
}

func (r *AnalysisReport) yearlyInterim(story analysisYearStory, sources map[string]analysisSource) reportSection {
	return reportSection{ID: "year-2026", Kind: "interim", PeriodLabel: "2026 interim",
		Title: "2026 interim — " + story.Title, Paragraphs: []reportCell{
			{Label: "At the bank", Text: story.Bank},
			{Label: "For shareholders", Text: "A 2026 share return is not shown because a matching dated price and dividend record has not been verified."},
			{Label: "Why it mattered", Text: story.Impact},
			{Label: "Sources", Links: yearlySourceLinks(sources, story.Sources, "", true)},
		}, Tables: []reportTable{r.interimEarningsTable(), r.interimRiskTable()}}
}

func (r *AnalysisReport) interimEarningsTable() reportTable {
	table := reportTable{Title: "Year-to-date earnings",
		Headers:        []string{"Measure", "2025 matched period", "2026 matched period", "Change"},
		NumericColumns: []int{1, 2, 3}, CurrentColumn: 2}
	for _, item := range []struct{ period, prior, basis, title string }{
		{"2026-09", "2025-09", "consolidated", "Group · January–September profit"},
		{"2026-10", "2025-10", "unconsolidated", "Bank only · January–October profit"},
	} {
		current, prior := r.interimPeriod(item.period, item.basis), r.interimPeriod(item.prior, item.basis)
		if current == nil {
			continue
		}
		currentProfit := yearlyFact(current.Facts["net_earnings"], "money", item.basis)
		priorProfit := yearlyValue{}
		if prior != nil {
			priorProfit = yearlyFact(prior.Facts["net_earnings"], "money", item.basis)
		}
		if currentProfit.valid {
			change := "—"
			if priorProfit.valid {
				change = yearlyChange(priorProfit, currentProfit, "money", item.period)
			}
			table.Rows = append(table.Rows, []reportCell{{Text: item.title}, yearlyCitationCell(priorProfit, "money"),
				yearlyCitationCell(currentProfit, "money"), {Text: change}})
		}
		if currentEPS := current.Values["earnings-per-share"]; currentEPS.Value != nil && prior != nil {
			curEPS := r.yearlyMetric(currentEPS, "eps")
			priorEPS := r.yearlyMetric(prior.Values["earnings-per-share"], "eps")
			if curEPS.valid {
				table.Rows = append(table.Rows, []reportCell{{Text: item.title + " · per share"}, yearlyCitationCell(priorEPS, "eps"),
					yearlyCitationCell(curEPS, "eps"), {Text: yearlyChange(priorEPS, curEPS, "eps", item.period)}})
			}
		}
	}
	return table
}

func (r *AnalysisReport) interimRiskTable() reportTable {
	table := reportTable{Title: "Selected group balance-sheet and risk indicators",
		Headers:        []string{"Measure", "Dec 2025", "Sep 2026", "Change"},
		NumericColumns: []int{1, 2, 3}, CurrentColumn: 2}
	for _, measure := range []struct{ key, unit, display string }{
		{"Gross loans and financing", "IDR billion", "Loans · group (IDR T)"},
		{"Capital adequacy ratio (KM1)", "%", "Capital adequacy · group"},
		{"Liquidity coverage ratio (KM1)", "%", "Liquidity coverage · group"},
		{"Net stable funding ratio (KM1)", "%", "Stable funding · group"},
	} {
		priorPeriod, currentPeriod := "2025-12", "2026-09"
		ticker := r.companyTicker()
		prior, current := r.findObservation(measure.key, ticker, priorPeriod, "consolidated", measure.unit), r.findObservation(measure.key, ticker, currentPeriod, "consolidated", measure.unit)
		if measure.key == "Gross loans and financing" {
			priorPeriod, currentPeriod = "2025", "2026-09"
			prior, current = r.findObservation(measure.key, ticker, priorPeriod, "consolidated", measure.unit), r.findObservation(measure.key, ticker, currentPeriod, "consolidated", measure.unit)
		}
		if current.Value == nil {
			continue
		}
		display, scale, unit := "percent", 1.0, "percent"
		if measure.unit == "IDR billion" {
			display, scale, unit = "money", 1e-3, "money"
		}
		priorValue := yearlyValue{}
		if prior.Value != nil {
			priorValue = yearlyValue{value: *prior.Value * scale, unit: display, basis: prior.Basis, links: r.observationLinks(prior), valid: true}
		}
		currentValue := yearlyValue{value: *current.Value * scale, unit: display, basis: current.Basis, links: r.observationLinks(current), valid: true}
		change := "level only"
		if priorValue.valid {
			change = yearlyChange(priorValue, currentValue, unit, currentPeriod)
		}
		table.Rows = append(table.Rows, []reportCell{{Text: measure.display}, yearlyCitationCell(priorValue, unit),
			yearlyCitationCell(currentValue, unit), {Text: change}})
	}
	return table
}

func (r *AnalysisReport) interimPeriod(period, basis string) *analysisPeriod {
	for i := range r.Interim {
		if r.Interim[i].Period == period && r.Interim[i].Basis == basis {
			return &r.Interim[i]
		}
	}
	return nil
}

func yearlyGlossary() reportSection {
	rows := [][]reportCell{}
	appendGroup := func(title string, entries ...[2]string) {
		rows = append(rows, []reportCell{{Text: title}, {}})
		for _, entry := range entries {
			rows = append(rows, textCells(entry[0], entry[1]))
		}
	}
	appendGroup("REPORTING SCOPE AND PERIODS",
		[2]string{"Group", "BCA and its subsidiaries, reported on a consolidated basis."},
		[2]string{"Bank only", "BCA parent bank without subsidiaries. Do not combine with group figures."},
		[2]string{"FY", "Full financial year. Profit and expenses cover the year; balance-sheet ratios describe a date."},
		[2]string{"YTD", "Year to date, compared with the same months of the prior year."},
		[2]string{"As filed / restated", "As-filed values preserve the original report. Restated values reflect later corrections or reclassifications."},
	)
	appendGroup("UNITS AND COMPARISON",
		[2]string{"IDR T", "Trillion Indonesian rupiah."},
		[2]string{"pp", "Percentage points: the arithmetic difference between two percentages."},
		[2]string{"×", "Times, used for valuation multiples such as price-to-earnings."},
		[2]string{"Change", "Money amounts show percentage growth; ratio measures use pp; EPS uses rupiah per share; multiples use ×. Negative-to-positive changes are stated in words."},
		[2]string{"Level only", "No comparable prior-period value is available on the same basis."},
	)
	appendGroup("BANK AND PERFORMANCE MEASURES",
		[2]string{"CASA", "Current and savings accounts; a lower-cost source of customer deposits."},
		[2]string{"NIM", "Net interest margin: net interest income relative to earning assets."},
		[2]string{"NPL / LAR", "Non-performing loans / loans at risk. These use different definitions; compare only matching scope and basis."},
		[2]string{"CAR", "Regulatory capital as a share of risk-weighted assets."},
		[2]string{"LCR / NSFR", "Liquidity coverage over a 30-day stress period / stable funding available against longer-term needs."},
		[2]string{"ROA / ROE", "Profit relative to average assets / average shareholders’ equity."},
	)
	appendGroup("SHAREHOLDER MEASURES",
		[2]string{"EPS", "Earnings per share. EPS and share-price figures use the October 2021 split-adjusted share basis."},
		[2]string{"P/E / P/B", "Share price compared with earnings per share / book value per share."},
		[2]string{"Share-price return", "Calendar-year change in split-adjusted share price; dividends excluded."},
		[2]string{"Total return", "Share-price change plus cash dividends, before tax and trading fees."},
		[2]string{"Dividend yield / payout", "Cash dividends relative to share price / earnings per share."},
		[2]string{"Real return", "Total return adjusted for Indonesian CPI inflation over the same calendar year."},
	)
	return reportSection{ID: "yearly-glossary", Kind: "glossary", Title: "Table legend and glossary",
		Paragraphs: []reportCell{{Text: "Definitions and calculation bases used in the year-by-year tables. Read each result with its reporting scope and period."}},
		Tables:     []reportTable{{Headers: []string{"Term", "Meaning in this report"}, Rows: rows}},
	}
}

func yearStoryLinks(sources map[string]analysisSource, keys []string, market string) []reportLink {
	links := make([]reportLink, 0, 3)
	seen := make(map[string]int)
	for _, key := range append(append([]string(nil), keys...), market) {
		if source, ok := sources[key]; ok && source.URL != "" {
			base, _, _ := strings.Cut(source.URL, "#")
			if index, exists := seen[base]; exists {
				if key == market {
					links[index].Text += " and share-price records"
					links[index].URL = source.URL
				}
				continue
			}
			if key == market {
				source.Title = "Share-price records"
			}
			links = append(links, reportLink{Text: source.Title, URL: source.URL})
			seen[base] = len(links) - 1
		}
		if len(links) == 3 {
			break
		}
	}
	return links
}

func yearlySourceLinks(sources map[string]analysisSource, storyKeys []string, market string, interim bool) []reportLink {
	links := yearStoryLinks(sources, storyKeys, market)
	keys := []struct{ key, title string }{
		{"income", "Audited statements"},
		{"dividend-record", "Dividend history"},
		{"ratios", "Bank ratios and asset quality"},
		{"car", "Capital and risk report"},
	}
	if interim {
		keys = []struct{ key, title string }{
			{"km1", "Capital and risk disclosure"},
			{"lar9m26", "September credit-risk presentation"},
		}
	}
	seen := make(map[string]bool)
	for _, link := range links {
		base, _, _ := strings.Cut(link.URL, "#")
		seen[base] = true
	}
	for _, item := range keys {
		source, ok := sources[item.key]
		if !ok || source.URL == "" {
			continue
		}
		base, _, _ := strings.Cut(source.URL, "#")
		if seen[base] {
			continue
		}
		seen[base] = true
		links = append(links, reportLink{Text: item.title, URL: source.URL})
		if len(links) >= 6 {
			break
		}
	}
	return links
}
