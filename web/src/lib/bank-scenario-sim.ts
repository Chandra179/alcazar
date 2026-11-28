import type {
  BankScenarioCase,
  BankScenarioResult,
  BankScenarios,
  ReportCell,
  ReportTable,
} from "@/types/analysis"

export interface BankSimulationInputs {
  commonEquity: number
  cet1: number
  rwa: number
  shares: number
  loans: number
  deposits: number
}

/** Extract real consolidated starting balances from `bank_scenarios.inputs`. */
export function extractScenarioInputs(scenarios?: BankScenarios): BankSimulationInputs | null {
  const obs = scenarios?.inputs
  if (!obs?.length) return null

  const byKey = new Map<string, number>()
  for (const item of obs) {
    if (item.key && typeof item.value === "number" && item.value > 0) {
      byKey.set(item.key, item.value)
    }
  }

  const commonEquity = byKey.get("Common equity attributable to parent")
  const cet1 = byKey.get("Common Equity Tier 1 capital")
  const rwa = byKey.get("Risk-weighted assets")
  const shares = byKey.get("Closing common shares ex treasury")
  const loans = byKey.get("Gross loans including group financing")
  const deposits = byKey.get("Customer deposits")

  if (!commonEquity || !cet1 || !rwa || !shares || !loans || !deposits) {
    return null
  }
  return { commonEquity, cet1, rwa, shares, loans, deposits }
}

function bankDividendValue(
  inputs: BankSimulationInputs,
  item: BankScenarioCase,
  discountOverride?: number,
) {
  const roe = item.normalized_roe ?? item.roe ?? 0
  const discount = discountOverride ?? item.cost_of_equity
  let earnings = inputs.commonEquity * roe
  let rwa = inputs.rwa
  let capital = inputs.cet1

  const forecast: { year: number; earnings: number; rwa: number; dividendPerShare: number }[] = []
  let value = 0

  for (let year = 1; year <= 5; year++) {
    earnings *= 1 + item.earnings_growth
    rwa *= 1 + item.rwa_growth
    const required = rwa * item.cet1_target
    const dividend = Math.min(earnings * item.payout, Math.max(0, capital + earnings - required))
    capital += earnings - dividend
    const dps = (dividend * 1e6) / inputs.shares
    value += dps / Math.pow(1 + discount, year)
    forecast.push({ year, earnings, rwa, dividendPerShare: dps })
  }

  const terminalEarnings = earnings * (1 + item.terminal_growth)
  const retention = item.cet1_target * rwa * item.terminal_growth
  const terminalDividend = Math.min(
    terminalEarnings * item.payout,
    Math.max(0, terminalEarnings - retention),
  )
  const shortfall = Math.max(0, item.cet1_target * rwa - capital)
  const terminalDPS = (terminalDividend * 1e6) / inputs.shares
  const denom = Math.max(0.0001, discount - item.terminal_growth)
  const terminalValue = Math.max(0, terminalDPS / denom - (shortfall * 1e6) / inputs.shares)

  return {
    value: value + terminalValue / Math.pow(1 + discount, 5),
    forecast,
  }
}

/** Port of Go `evaluateBankScenario` for live in-browser recalculation. */
export function evaluateBankScenario(
  item: BankScenarioCase,
  inputs: BankSimulationInputs,
): BankScenarioResult {
  const roe = item.normalized_roe ?? item.roe ?? 0
  const base = bankDividendValue(inputs, item)
  const higherRate = bankDividendValue(inputs, item, item.cost_of_equity + 0.01)

  let book = inputs.commonEquity
  let residual = inputs.commonEquity
  for (const yr of base.forecast) {
    residual +=
      (yr.earnings - item.cost_of_equity * book) / Math.pow(1 + item.cost_of_equity, yr.year)
    book += yr.earnings - (yr.dividendPerShare * inputs.shares) / 1e6
  }
  const last = base.forecast[base.forecast.length - 1]
  const terminalIncome = last.earnings * (1 + item.terminal_growth)
  const denom = Math.max(0.0001, item.cost_of_equity - item.terminal_growth)
  residual +=
    (terminalIncome - item.cost_of_equity * book) /
    denom /
    Math.pow(1 + item.cost_of_equity, base.forecast.length)

  const stress = item.one_year_stress
  const credit = stress
    ? inputs.loans * stress.affected_loan_share * stress.incremental_loss_severity
    : 0
  const funding = stress
    ? inputs.deposits *
      ((1 - stress.deposit_runoff_share) *
        stress.deposit_repricing_share *
        stress.policy_rate_increase *
        stress.deposit_beta +
        stress.deposit_runoff_share * stress.replacement_funding_spread)
    : 0
  const taxRate = stress?.tax_rate ?? 0
  const rwaIncrease = stress?.rwa_increase ?? 0
  const cet1Adjustment = stress?.cet1_adjustment_idr_million ?? 0

  const earnings = base.forecast[0].earnings - (credit + funding) * (1 - taxRate)
  const rwa = base.forecast[0].rwa * (1 + rwaIncrease)
  const required = rwa * item.cet1_target
  const available = inputs.cet1 + earnings + cet1Adjustment
  const payout = Math.min(Math.max(0, earnings * item.payout), Math.max(0, available - required))
  const capital = available - payout

  return {
    assumptions: item,
    normalized_common_earnings_idr_million: inputs.commonEquity * roe,
    dividend_value_per_share: base.value,
    dividend_value_at_cost_plus_1pp: higherRate.value,
    residual_income_value_per_share: (residual * 1e6) / inputs.shares,
    forecast: base.forecast.map((yr) => ({
      year: yr.year,
      net_income: yr.earnings,
      dividend_per_share: yr.dividendPerShare,
      rwa: yr.rwa,
      cet1: 0,
    })),
    incremental_credit_loss_idr_million: credit,
    incremental_funding_cost_idr_million: funding,
    stressed_cet1_idr_million: capital,
    stressed_rwa_idr_million: rwa,
    cet1_headroom_idr_million: capital - required,
  }
}

function cells(...texts: string[]): ReportCell[] {
  return texts.map((text) => ({ text }))
}

/** Build the `One-year stress` and `Valuation in rupiah per share` tables from live results. */
export function buildScenarioOutputTables(
  results: BankScenarioResult[],
  baselineResults?: BankScenarioResult[],
  currencySymbol = "Rp",
): ReportTable[] {
  const stress: ReportTable = {
    title: "One-year stress",
    headers: ["Case", "Credit hit", "Funding hit", "Closing CET1 / target", "CET1 gap vs target"],
    numeric_columns: [1, 2, 3, 4],
    rows: [],
  }

  const values: ReportTable = {
    title: "Valuation in rupiah per share",
    headers: [
      "Case",
      "Dividend value",
      "Residual income",
      "Normalized earnings",
      "Year 1 dividend",
    ],
    numeric_columns: [1, 2, 3, 4],
    rows: [],
  }

  for (const row of results) {
    const item = row.assumptions ?? row.case
    if (!item) continue
    const divVal = row.dividend_value_per_share ?? row.dividend_value ?? 0
    const resVal = row.residual_income_value_per_share ?? row.residual_value ?? 0
    const normInc = row.normalized_common_earnings_idr_million ?? row.normalized_income ?? 0
    const y1Dps =
      (row.forecast?.[0] as { dividend_idr_per_share?: number; dividend_per_share?: number } | undefined)
        ?.dividend_idr_per_share ??
      row.forecast?.[0]?.dividend_per_share ??
      0
    const credit = row.incremental_credit_loss_idr_million ?? row.credit_loss ?? 0
    const funding = row.incremental_funding_cost_idr_million ?? row.funding_cost ?? 0
    const sCet1 = row.stressed_cet1_idr_million ?? row.stressed_cet1 ?? 0
    const sRwa = row.stressed_rwa_idr_million ?? row.stressed_rwa ?? 1
    const headroom = row.cet1_headroom_idr_million ?? row.cet1_headroom ?? 0

    const gapT = headroom / 1e6
    const gapText = `${gapT >= 0 ? "+" : ""}${gapT.toFixed(2)}T`

    stress.rows.push(
      cells(
        item.name,
        `${(credit / 1e6).toFixed(2)}T`,
        `${(funding / 1e6).toFixed(2)}T`,
        `${((100 * sCet1) / sRwa).toFixed(2)}% / ${(item.cet1_target * 100).toFixed(0)}%`,
        gapText,
      ),
    )

    values.rows.push(
      cells(
        item.name,
        `${currencySymbol}${Math.round(divVal)}`,
        `${currencySymbol}${Math.round(resVal)}`,
        `${(normInc / 1e6).toFixed(2)}T`,
        `${currencySymbol}${Math.round(y1Dps)}`,
      ),
    )
  }

  if (baselineResults?.length) {
    const baseTables = buildScenarioOutputTables(baselineResults, undefined, currencySymbol)
    ;[stress, values].forEach((table, tIdx) => {
      const baseTable = baseTables[tIdx]
      table.rows.forEach((row, rIdx) => {
        const baseRow = baseTable?.rows[rIdx]
        if (!baseRow) return
        row.forEach((cell, cIdx) => {
          if (cIdx > 0 && baseRow[cIdx] && cell.text !== baseRow[cIdx].text) {
            cell.changed = true
          }
        })
      })
    })
  }

  return [stress, values]
}
