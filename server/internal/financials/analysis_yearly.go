package financials

import (
	"fmt"
	"math"
	"strings"
)

type analysisYearStory struct {
	Title   string   `yaml:"title"`
	Bank    string   `yaml:"bank"`
	Impact  string   `yaml:"impact"`
	Sources []string `yaml:"sources"`
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
	sections := []reportSection{{Paragraphs: []reportCell{
		{Text: "Bank Central Asia (BBCA) from the Asian financial crisis through the latest reviewed 2026 disclosures."},
		{Text: "Share-price changes compare year-end closes after adjusting for stock splits. Annual returns including dividends assume you held the shares all year. They count dividends as cash and are before taxes and trading fees.",
			Links: yearStoryLinks(sources, []string{"dividend-record", "split"}, "")},
	}}}
	interim, ok := stories[interimYear]
	if !ok {
		return nil, fmt.Errorf("missing yearly story for %s", interimYear)
	}
	sections = append(sections, r.yearlyInterim(interim, sources))
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
		sections = append(sections, reportSection{
			ID: "year-" + row.Period, Title: row.Period + " — " + story.Title,
			Paragraphs: []reportCell{
				{Label: "At the bank", Text: story.Bank + profitSummary(row.Facts["net_earnings"], prior)},
				{Label: "For shareholders", Text: r.yearlyStockSummary(row, close)},
				{Label: "Why it mattered", Text: story.Impact},
				{Links: yearStoryLinks(sources, story.Sources, close.Source)},
			},
		})
	}
	return sections, nil
}

func profitSummary(current, prior Fact) string {
	profit, ok := annualProfitInTrillions(current)
	if !ok {
		return ""
	}
	if profit < 0 {
		return fmt.Sprintf(" The group lost Rp%.1f trillion.", -profit)
	}
	previous, ok := annualProfitInTrillions(prior)
	if !ok || prior.Period != previousBalancePeriod(current.Period) ||
		current.Period == bbcaListingYear || prior.Period == bbcaListingYear || previous == 0 {
		return fmt.Sprintf(" Group profit was Rp%.1f trillion.", profit)
	}
	if previous < 0 && profit > 0 {
		return fmt.Sprintf(" BCA returned to profit, earning Rp%.1f trillion.", profit)
	}
	change := (profit/previous - 1) * 100
	if math.Abs(change) < 0.05 {
		return fmt.Sprintf(" Group profit was roughly unchanged at Rp%.1f trillion.", profit)
	}
	return fmt.Sprintf(" Group profit %s to Rp%.1f trillion.", changeInWords(change), profit)
}

func annualProfitInTrillions(fact Fact) (float64, bool) {
	value, ok := parseCanonicalNumber(fact.NormalizedValue)
	scale, supported := unitScale(fact.Unit)
	return value * scale / 1e12, ok && supported && isMoneyScale(fact.Unit) && fact.Currency == "IDR" &&
		fact.ReportingBasis == "consolidated" && fact.AuditedStatus == "audited" && fact.PeriodType == "annual" && samePeriodFlow(fact)
}

func (r *AnalysisReport) yearlyStockSummary(row analysisPeriod, close analysisObservation) string {
	if row.Period < bbcaListingYear {
		return "Before listing. BBCA shares did not yet trade on the exchange."
	}
	if row.Period == bbcaListingYear {
		listing := "The listing year. BBCA began trading in May, so there is no full-calendar return."
		if close.Value != nil && *close.Value > 0 {
			return listing + fmt.Sprintf(" The verified year-end price was Rp%.3f on the split-adjusted scale.", *close.Value)
		}
		return listing
	}
	open := r.marketShareObservation("Year-end close", previousBalancePeriod(row.Period))
	if open.Value == nil || *open.Value <= 0 || close.Value == nil || *close.Value <= 0 {
		return "A comparable full-year share-price change is unavailable in the reviewed records."
	}
	price := 100 * (*close.Value - *open.Value) / *open.Value
	if total := row.Values["nominal-shareholder-return"].Value; total != nil {
		return fmt.Sprintf("The share price %s. Return with dividends was %.1f%%.", changeInWords(price), *total)
	}
	return fmt.Sprintf("The share price %s. Return with dividends is unavailable in the reviewed records.", changeInWords(price))
}

func changeInWords(percent float64) string {
	if math.Abs(percent) < 0.05 {
		return "was roughly unchanged"
	}
	if percent < 0 {
		return fmt.Sprintf("fell %.1f%%", -percent)
	}
	return fmt.Sprintf("rose %.1f%%", percent)
}

func (r *AnalysisReport) yearlyInterim(story analysisYearStory, sources map[string]analysisSource) reportSection {
	bank := []string{story.Bank}
	for _, item := range []struct{ period, basis, label string }{
		{"2026-09", "consolidated", "Through September, profit for BCA and its subsidiaries"},
		{"2026-10", "unconsolidated", "Through October, profit for BCA bank only"},
	} {
		for _, row := range r.Interim {
			if row.Period == item.period && row.Basis == item.basis {
				if growth := row.Values["interim-net-earnings-yoy"].Value; growth != nil {
					bank = append(bank, fmt.Sprintf("%s %s compared with the same months a year earlier.", item.label, changeInWords(*growth)))
				}
				break
			}
		}
	}
	stock := "The reviewed snapshot has no matching share price and dividend record for a return through October."
	open := r.marketShareObservation("Year-end close", "2025")
	close := r.marketShareObservation("Period-end close", "2026-10")
	dividends := r.marketShareObservation("Dividends earned", "2026-10")
	if open.Value != nil && *open.Value > 0 && close.Value != nil && *close.Value > 0 && dividends.Value != nil && *dividends.Value >= 0 {
		price := 100 * (*close.Value - *open.Value) / *open.Value
		stock = fmt.Sprintf("Through October, the share price %s. Return with dividends was %.1f%%.",
			changeInWords(price), shareholderReturn(*open.Value, *close.Value, *dividends.Value)*100)
	}
	return reportSection{ID: "year-2026", Title: "2026 so far — " + story.Title, Paragraphs: []reportCell{
		{Label: "At the bank", Text: strings.Join(bank, " ")},
		{Label: "For shareholders", Text: stock},
		{Label: "Why it mattered", Text: story.Impact},
		{Links: yearStoryLinks(sources, story.Sources, "")},
	}}
}

func yearStoryLinks(sources map[string]analysisSource, keys []string, market string) []reportLink {
	links := make([]reportLink, 0, 2)
	for _, key := range append(append([]string(nil), keys...), market) {
		if source, ok := sources[key]; ok && (len(links) == 0 || links[0].URL != source.URL) {
			if key == market {
				source.Title = "Share-price records"
			}
			links = append(links, reportLink{Text: source.Title, URL: source.URL})
		}
		if len(links) == 2 {
			break
		}
	}
	return links
}
