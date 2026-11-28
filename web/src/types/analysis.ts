export interface ReportLink {
  text: string
  url: string
}

export interface ReportCell {
  text: string
  links?: ReportLink[]
  label?: string
  changed?: boolean
}

export interface TableLayout {
  contents?: boolean
  column_roles: string[]
  group_rows?: number[]
}

export interface ReportTable {
  title?: string
  headers: string[]
  rows: ReportCell[][]
  numeric_columns?: number[]
  current_column?: number
  layout?: TableLayout
}

export interface SectionLayout {
  takeaways?: boolean
  source_rows?: number[]
}

export interface ReportSection {
  id?: string
  title?: string
  kind?: string
  period_label?: string
  paragraphs?: ReportCell[]
  tables?: ReportTable[]
  layout?: SectionLayout
}

export interface YearlyTemplate {
  id: string
  title: string
  footer: string
}

export interface AnalysisSource {
  title: string
  url: string
  locator: string
}

export interface CatalogRule {
  id: string
  category?: string
  title?: string
  question?: string
}

export interface AnalysisRule {
  number: number
  rule: CatalogRule
  status: string
  finding: string
  gap?: string
  sources: string[]
}

export interface AnalysisSectionSummary {
  title: string
  status: string
  finding: string
  gap?: string
  sources: string[]
}

export interface BankStressAssumptions {
  affected_loan_share: number
  incremental_loss_severity: number
  policy_rate_increase: number
  deposit_beta: number
  deposit_repricing_share: number
  deposit_runoff_share: number
  replacement_funding_spread: number
  rwa_increase: number
  cet1_adjustment_idr_million: number
  tax_rate: number
}

export interface BankScenarioCase {
  name: string
  normalized_roe?: number
  roe?: number
  earnings_growth: number
  rwa_growth: number
  payout: number
  cost_of_equity: number
  terminal_growth: number
  cet1_target: number
  one_year_stress?: BankStressAssumptions
}

export interface BankScenarioForecast {
  year: number
  net_income: number
  dividend_per_share: number
  rwa: number
  cet1: number
}

export interface BankScenarioResult {
  assumptions?: BankScenarioCase
  case?: BankScenarioCase
  dividend_value_per_share?: number
  dividend_value?: number
  dividend_value_at_cost_plus_1pp?: number
  dividend_value_higher_rate?: number
  residual_income_value_per_share?: number
  residual_value?: number
  normalized_common_earnings_idr_million?: number
  normalized_income?: number
  incremental_credit_loss_idr_million?: number
  credit_loss?: number
  incremental_funding_cost_idr_million?: number
  funding_cost?: number
  stressed_cet1_idr_million?: number
  stressed_cet1?: number
  stressed_rwa_idr_million?: number
  stressed_rwa?: number
  cet1_headroom_idr_million?: number
  cet1_headroom?: number
  forecast: BankScenarioForecast[]
}

export interface GlossaryEntry {
  term: string
  meaning: string
  group?: string
}

export interface BankScenarios {
  period: string
  inputs?: { key?: string; value?: number | null; source: string }[]
  results: BankScenarioResult[]
}

export interface PeerComparisonRow {
  bank: string
  [key: string]: unknown
}

export interface AnalysisReport {
  catalog_hash: string
  facts_hash: string
  reviewed_catalog_hash: string
  bank_catalog_hash?: string
  yearly_template?: YearlyTemplate
  yearly_sections?: ReportSection[]
  bank_scenarios?: BankScenarios
  rules?: AnalysisRule[]
  sections?: AnalysisSectionSummary[]
  sources?: Record<string, AnalysisSource>
  peer_comparisons?: PeerComparisonRow[]
}

export interface AnnualPoint {
  year: number
  label: string
  profit: number | null
  loans: number | null
  deposits: number | null
  casa: number | null
  nim: number | null
  roe: number | null
  roa: number | null
  npl: number | null
  car: number | null
  divYield: number | null
  pe: number | null
  pbv: number | null
  totalReturn: number | null
  profitIndex: number | null
  returnIndex: number | null
}
