package financials

// addAnnualObservations fills calculations with reviewed disclosures tied to the same period and basis.
func addAnnualObservations(report *AnalysisReport) {
	for _, row := range report.Annual {
		period := row.Period
		find := func(key, basis, unit string) analysisObservation {
			return report.findObservation(key, "BBCA", period, basis, unit)
		}
		if row.Values["net-change-in-cash"].Status == "missing_inputs" {
			operating := find("Operating cash flow", "consolidated", "IDR million")
			investing := find("Investing cash flow", "consolidated", "IDR million")
			financing := find("Financing cash flow", "consolidated", "IDR million")
			if operating.Value != nil && investing.Value != nil && financing.Value != nil {
				row.Values["net-change-in-cash"] = derived("net-change-in-cash", period, "IDR millions",
					*operating.Value+*investing.Value+*financing.Value,
					map[string]any{"operating": operating, "investing": investing, "financing": financing})
			}
		}
		net := find("Net income (operating-income presentation)", "consolidated", "IDR billion")
		income := find("Reported operating income", "consolidated", "IDR billion")
		if row.Values["net-profit-margin"].Status == "missing_inputs" && net.Value != nil && income.Value != nil &&
			*income.Value > 0 && net.Source == income.Source {
			row.Values["net-profit-margin"] = derived("net-profit-margin", period, "%", *net.Value / *income.Value * 100,
				map[string]any{"reported_net_earnings": net, "reported_operating_income": income})
		}

		shares := find("Split-adjusted weighted average shares", "consolidated", "shares")
		earnings := row.Facts["net_income_available_to_common_shareholders"]
		if n, ok := parseCanonicalNumber(earnings.NormalizedValue); row.Values["earnings-per-share"].Status == "missing_inputs" &&
			ok && earnings.Unit == "millions" && earnings.Currency == "IDR" && earnings.AuditedStatus == "audited" &&
			shares.Value != nil && *shares.Value > 0 && shares.ShareBasis == analysisShareBasis {
			row.Values["earnings-per-share"] = derived("earnings-per-share", period, "currency per share", n*1e6 / *shares.Value,
				map[string]any{"parent_earnings": earnings, "split_adjusted_weighted_average_shares": shares})
		}
		if _, matched := matchingAnnualEPS(report, row); !matched {
			adjusted := find("Basic EPS on 2025 share basis", "consolidated", "IDR/share")
			if adjusted.Value != nil && adjusted.ShareBasis == analysisShareBasis {
				row.Values["earnings-per-share"] = derived("earnings-per-share", period, "currency per share", *adjusted.Value,
					map[string]any{"reported_eps_on_split_adjusted_basis": adjusted})
			}
		}

		for id, metric := range annualMarketMetrics(report, row, previousBalancePeriod(period)) {
			row.Values[id] = metric
		}
	}
}

func addInterimMargins(report *AnalysisReport) {
	for _, row := range report.Interim {
		if row.Basis != "consolidated" {
			continue
		}
		bank, income, ok := report.operatingIncome(row.Period)
		earnings := row.Facts["net_earnings"]
		net, valid := parseCanonicalNumber(earnings.NormalizedValue)
		if !ok || !valid || earnings.Unit != "millions" || earnings.Currency != "IDR" || !samePeriodFlow(earnings) {
			continue
		}
		for _, item := range []struct {
			id      string
			revenue float64
			inputs  map[string]any
		}{
			{"bank-net-income-to-operating-income", bank.value, bank.inputs},
			{"net-profit-margin", income.value, income.inputs},
		} {
			if item.revenue > 0 && row.Values[item.id].Value == nil {
				item.inputs["group_net_earnings"] = earnings
				row.Values[item.id] = derived(item.id, row.Period, "%", net/item.revenue*100, item.inputs)
			}
		}
	}
}

type operatingIncome struct {
	value  float64
	inputs map[string]any
}

func (r *AnalysisReport) operatingIncome(period string) (bank, group operatingIncome, ok bool) {
	find := func(key string) analysisObservation {
		return r.findObservation(key, "BBCA", period, "consolidated", "IDR million")
	}
	interest, other, insurance := find("Net interest and sharia income"), find("Other operating income"), find("Net insurance income")
	if interest.Value == nil || other.Value == nil || insurance.Value == nil ||
		interest.Source != other.Source || interest.Source != insurance.Source {
		return bank, group, false
	}
	bank = operatingIncome{*interest.Value + *other.Value, map[string]any{"net_interest_and_sharia_income": interest, "other_operating_income": other}}
	group = operatingIncome{bank.value + *insurance.Value,
		map[string]any{"net_interest_and_sharia_income": interest, "other_operating_income": other, "net_insurance_income": insurance}}
	return bank, group, true
}

const analysisShareBasis = "october-2021-split"
const bbcaListingYear = "2000"

func (r *AnalysisReport) marketShareObservation(key, period string) analysisObservation {
	item := r.findObservation(key, "BBCA", period, "market", "IDR/share")
	if item.ShareBasis != analysisShareBasis {
		return analysisObservation{}
	}
	return item
}

func shareholderReturn(open, close, dividends float64) float64 {
	return (close+dividends)/open - 1
}

func matchingAnnualEPS(report *AnalysisReport, row analysisPeriod) (Metric, bool) {
	exact := row.Values["earnings-per-share"]
	shares := report.findObservation("Split-adjusted weighted average shares", "BBCA", row.Period, "consolidated", "shares")
	if exact.Value == nil || shares.ShareBasis != analysisShareBasis || shares.Value == nil || *shares.Value <= 0 {
		return exact, false
	}
	if input, ok := exact.Inputs["split_adjusted_weighted_average_shares"].(analysisObservation); ok {
		return exact, input.ShareBasis == shares.ShareBasis && input.Period == shares.Period &&
			input.Basis == shares.Basis && input.Value != nil && *input.Value == *shares.Value
	}
	input, _ := exact.Inputs["weighted_average_common_shares_outstanding"].(map[string]any)
	value, ok := input["value"].(float64)
	return exact, ok && input["unit"] == "shares" && value == *shares.Value
}

func annualMarketMetrics(report *AnalysisReport, row analysisPeriod, prior string) map[string]Metric {
	period := row.Period
	find := func(key, year string) analysisObservation {
		return report.marketShareObservation(key, year)
	}
	results := make(map[string]Metric)
	for id, reason := range map[string]string{
		"price-earnings-ratio":        "Requires a reviewed year-end close and positive basic EPS on the same split basis.",
		"price-to-book-ratio":         "Requires a reviewed year-end close and positive book value per common share on the same split basis.",
		"dividend-yield":              "Requires a reviewed year-end close and cash dividends paid during the calendar year on the same split basis.",
		"real-return-after-inflation": "Requires two year-end closes, dividends earned and national CPI growth for the same holding period.",
		"nominal-shareholder-return":  "Requires two year-end closes and dividends earned during the holding period.",
		"fiscal-dividend-payout":      "Requires reviewed fiscal dividends and positive EPS on the same split basis.",
	} {
		status := "missing_inputs"
		if period < bbcaListingYear {
			status, reason = "not_applicable", "BBCA listed in 2000."
		}
		results[id] = Metric{EntityID: id, Period: period, Status: status, Reason: reason}
	}
	if period == bbcaListingYear {
		for _, id := range []string{"nominal-shareholder-return", "real-return-after-inflation"} {
			results[id] = Metric{EntityID: id, Period: period, Status: "not_applicable",
				Reason: "BBCA listed during 2000, so a full-calendar-year holding return is unavailable."}
		}
	}
	eps := report.findObservation("Basic EPS on 2025 share basis", "BBCA", period, "consolidated", "IDR/share")
	var epsValue float64
	var epsInput any
	if eps.Value != nil && eps.ShareBasis == analysisShareBasis {
		epsValue, epsInput = *eps.Value, eps
	}
	if exact, ok := matchingAnnualEPS(report, row); ok {
		epsValue, epsInput = *exact.Value, exact
	}
	fiscal := find("Fiscal dividends", period)
	if fiscal.Value != nil && *fiscal.Value >= 0 && epsValue > 0 {
		results["fiscal-dividend-payout"] = derived("fiscal-dividend-payout", period, "%",
			*fiscal.Value/epsValue*100, map[string]any{"dps": fiscal, "eps": epsInput})
	}
	close := find("Year-end close", period)
	if close.Value == nil || *close.Value <= 0 {
		return results
	}
	if epsValue > 0 {
		results["price-earnings-ratio"] = derived("price-earnings-ratio", period, "×", *close.Value/epsValue,
			map[string]any{"close": close, "eps": epsInput})
	}
	book := find("Published book value per share", period)
	shares := report.findObservation("Closing common shares ex treasury", "BBCA", period, "consolidated", "shares")
	equity := row.Facts["common_shareholders_equity"]
	equityValue, equityOK := parseCanonicalNumber(equity.NormalizedValue)
	scale, scaleOK := unitScale(equity.Unit)
	if equityOK && scaleOK && equityValue > 0 && shares.Value != nil && *shares.Value > 0 && shares.ShareBasis == analysisShareBasis {
		bvps := equityValue * scale / *shares.Value
		results["price-to-book-ratio"] = derived("price-to-book-ratio", period, "×", *close.Value/bvps,
			map[string]any{"close": close, "common_equity": equity, "shares": shares, "book_value_per_share": bvps})
	} else if book.Value != nil && *book.Value > 0 {
		results["price-to-book-ratio"] = derived("price-to-book-ratio", period, "×", *close.Value / *book.Value,
			map[string]any{"close": close, "published_book_value_per_share": book})
	}
	paid := find("Cash dividends paid", period)
	if paid.Value != nil && *paid.Value >= 0 {
		results["dividend-yield"] = derived("dividend-yield", period, "%", *paid.Value / *close.Value * 100,
			map[string]any{"close": close, "cash_dividends_paid": paid})
	}
	if period <= bbcaListingYear {
		return results
	}
	open, earned := find("Year-end close", prior), find("Dividends earned", period)
	if open.Value != nil && *open.Value > 0 && earned.Value != nil && *earned.Value >= 0 {
		nominal := shareholderReturn(*open.Value, *close.Value, *earned.Value)
		nominalMetric := derived("nominal-shareholder-return", period, "%", nominal*100,
			map[string]any{"opening_close": open, "ending_close": close, "dividends_earned": earned})
		results["nominal-shareholder-return"] = nominalMetric
		inflation := report.findObservation("CPI growth", "BBCA", period, "Indonesia CPI", "%")
		if inflation.Value != nil && *inflation.Value > -100 {
			results["real-return-after-inflation"] = derived("real-return-after-inflation", period, "%",
				((1+nominal)/(1+*inflation.Value/100)-1)*100, map[string]any{"nominal_return": nominalMetric, "inflation": inflation})
		}
	}
	return results
}
