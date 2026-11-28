package financials

import (
	"fmt"
	"strings"
)

type formulaDefinition struct {
	inputs               []string
	unit, status, reason string
	compute              func(map[string]periodFact) (float64, error)
	openingBalance       bool
	years                int
}

func ratioDefinition(numerator, denominator, unit string, multiplier float64) formulaDefinition {
	return formulaDefinition{inputs: []string{numerator, denominator}, unit: unit, compute: func(v map[string]periodFact) (float64, error) {
		if v[denominator].Number <= 0 {
			return 0, fmt.Errorf("%s must be positive", denominator)
		}
		return v[numerator].Number / v[denominator].Number * multiplier, nil
	}}
}

var formulaDefinitions = func() map[string]formulaDefinition {
	assets := ratioDefinition("net_earnings", "average_total_assets", "percent", 100)
	equity := ratioDefinition("net_income_available_to_common_shareholders", "average_common_shareholders_equity", "percent", 100)
	assets.openingBalance, equity.openingBalance = true, true
	definitions := map[string]formulaDefinition{
		"net-profit-margin":                   ratioDefinition("net_earnings", "revenue", "percent", 100),
		"effective-tax-rate":                  ratioDefinition("income_tax_expense", "income_before_tax", "percent", 100),
		"total-liabilities-to-equity":         ratioDefinition("total_liabilities", "shareholders_equity", "ratio", 1),
		"return-on-assets":                    assets,
		"return-on-equity":                    equity,
		"earnings-per-share":                  {inputs: []string{"net_income_available_to_common_shareholders", "weighted_average_common_shares_outstanding"}, unit: "currency per share", compute: earningsPerShare},
		"net-change-in-cash":                  {inputs: []string{"net_cash_from_operating_activities", "net_cash_from_investing_activities", "net_cash_from_financing_activities"}, unit: "source currency and scale", compute: sumCashMovements},
		"capital-expenditure-to-net-earnings": {inputs: []string{"capital_expenditures", "net_earnings"}, unit: "percent", years: 10},
	}
	for id := range notApplicableBankFormulas {
		definitions[id] = formulaDefinition{status: "not_applicable", reason: "This catalog formula is for a different business or security type and does not map reliably to a deposit-taking bank."}
	}
	for id, reason := range formulasRequiringExternalData {
		definitions[id] = formulaDefinition{status: "requires_external_data", reason: reason}
	}
	for id, reason := range formulasRequiringManualEstimate {
		definitions[id] = formulaDefinition{status: "requires_manual_estimate", reason: reason}
	}
	return definitions
}()

func earningsPerShare(v map[string]periodFact) (float64, error) {
	shares := v["weighted_average_common_shares_outstanding"].Number
	if shares <= 0 {
		return 0, fmt.Errorf("share count must be positive")
	}
	return numberAtUnit(v["net_income_available_to_common_shareholders"], "units") / shares, nil
}

func sumCashMovements(v map[string]periodFact) (float64, error) {
	return v["net_cash_from_operating_activities"].Number + v["net_cash_from_investing_activities"].Number + v["net_cash_from_financing_activities"].Number, nil
}

const primaryAnalysisSector = "deposit-taking-bank"

var notApplicableBankFormulas = map[string]bool{
	"cost-of-goods-sold":              true,
	"gross-margin":                    true,
	"gross-profit":                    true,
	"operating-profit":                true,
	"sga-to-gross-profit":             true,
	"rd-to-gross-profit":              true,
	"depreciation-to-gross-profit":    true,
	"interest-to-operating-income":    true,
	"net-receivables-to-sales":        true,
	"return-on-invested-capital":      true,
	"working-capital":                 true,
	"net-current-asset-value":         true,
	"current-ratio":                   true,
	"quick-ratio":                     true,
	"free-cash-flow":                  true,
	"insurance-float-cost":            true,
	"closed-end-fund-discount-to-nav": true,
}

var formulasRequiringExternalData = map[string]string{
	"price-earnings-ratio":            "Requires a labeled market share price and EPS basis, which are outside the local financial-statement data.",
	"price-to-book-ratio":             "Requires a market price per share, which are outside the local financial-statement data.",
	"dividend-yield":                  "Requires a labeled trailing or expected annual dividend per share and market share price.",
	"dividend-payout-ratio":           "Requires a labeled annual or trailing cash dividend per share matched to the EPS period.",
	"earnings-yield-on-original-cost": "Requires investor-specific split-adjusted purchase price and normalized earnings per share.",
	"real-return-after-inflation":     "Requires a nominal return and a matching-period inflation rate.",
	"look-through-earnings":           "Requires investee earnings not recognized in reported income and an estimate of incremental distribution taxes.",
}

var formulasRequiringManualEstimate = map[string]string{
	"owner-earnings":                   "Requires normalized earnings, maintenance capital expenditures, and incremental working-capital estimates that are not directly reported facts.",
	"return-on-tangible-common-equity": "Requires separating goodwill and other intangible assets from common equity on a consistent basis.",
}

// The catalog documents the formulas. Match each expression before using its Go implementation.
var implementedExpressions = map[string]string{
	"net-profit-margin":                   "(net_earnings / revenue) * 100",
	"effective-tax-rate":                  "(income_tax_expense / income_before_tax) * 100",
	"total-liabilities-to-equity":         "total_liabilities / shareholders_equity",
	"return-on-assets":                    "(net_earnings / average_total_assets) * 100",
	"return-on-equity":                    "(net_income_available_to_common_shareholders / average_common_shareholders_equity) * 100",
	"earnings-per-share":                  "net_income_available_to_common_shareholders / weighted_average_common_shares_outstanding",
	"net-change-in-cash":                  "net_cash_from_operating_activities + net_cash_from_investing_activities + net_cash_from_financing_activities",
	"capital-expenditure-to-net-earnings": "(sum(capital_expenditures over period) / sum(net_earnings over same period)) * 100",
	"price-earnings-ratio":                "share_price / earnings_per_share",
	"price-to-book-ratio":                 "market_price_per_share / book_value_per_common_share",
	"dividend-yield":                      "(annual_dividends_per_share / share_price) * 100",
	"real-return-after-inflation":         "(((1 + nominal_return_rate) / (1 + inflation_rate)) - 1) * 100",
}

func formulaMatches(entry CatalogEntry) bool {
	if entry.Type == "formula" {
		for _, sector := range entry.ExcludeSectors {
			if sector == primaryAnalysisSector && formulaDefinitions[entry.ID].status != "not_applicable" {
				return false
			}
		}
	}
	expression, implemented := implementedExpressions[entry.ID]
	return !implemented || entry.Type == "formula" && strings.Join(strings.Fields(expression), "") == strings.Join(strings.Fields(entry.Formula), "")
}
