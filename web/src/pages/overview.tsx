import { ChevronDown, Info, Table2, Upload } from "lucide-react"
import { useMemo, useState } from "react"

import { IndexedCompoundingChart } from "@/components/charts/indexed-compounding-chart"
import { ProfitRoeChart } from "@/components/charts/profit-roe-chart"
import { FinancialTable } from "@/components/financial-table"
import { AuditSection } from "@/components/sections/audit-section"
import { HistorySection } from "@/components/sections/history-section"
import { RatiosSection } from "@/components/sections/ratios-section"
import { RiskStressSection } from "@/components/sections/risk-stress-section"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useReport } from "@/lib/report-context"
import {
  extractAnnualSeries,
  extractGlossaryTerms,
  extractRowDelta,
  extractRowValue,
  getReportTicker,
  getSectionById,
  getSectionsByKind,
} from "@/lib/report-data"

function KpiInfoTooltip({ text }: { text: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={text}
          className="inline-flex shrink-0 items-center text-muted-foreground/70 hover:text-foreground focus-visible:outline-none"
        >
          <Info className="size-3" />
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{text}</TooltipContent>
    </Tooltip>
  )
}

function splitTitleSummary(title?: string): { label: string; summary?: string } {
  if (!title) return { label: "" }
  const match = title.match(/^(.+?)\s+[—-]\s+(.+)$/)
  if (!match) return { label: title }
  return { label: match[1].trim(), summary: match[2].trim() }
}

export function OverviewPage() {
  const [span, setSpan] = useState<"5" | "10" | "all">("10")
  const [chartMode, setChartMode] = useState<"profit" | "compounding">("profit")
  const [showInterimTables, setShowInterimTables] = useState(false)
  const { report, loading, openFilePicker } = useReport()

  const ticker = report ? getReportTicker(report) : ""
  const allSeries = useMemo(() => (report ? extractAnnualSeries(report) : []), [report])
  const basePoint = useMemo(
    () => allSeries.find((p) => p.profitIndex !== null) ?? allSeries[0],
    [allSeries],
  )
  const chartSeries = useMemo(() => {
    const base = basePoint ? allSeries.filter((p) => p.year >= basePoint.year) : allSeries
    if (span === "5") return base.slice(-5)
    if (span === "10") return base.slice(-10)
    return base
  }, [allSeries, basePoint, span])

  const annualSections = useMemo(
    () => (report ? getSectionsByKind(report, "annual") : []),
    [report],
  )
  const interimSections = useMemo(
    () => (report ? getSectionsByKind(report, "interim") : []),
    [report],
  )
  const latestAnnual = annualSections[0]
  const latestInterim = interimSections[0]
  const bankRisk = report ? getSectionById(report, "yearly-bank-risk") : undefined
  const scenarioSection = report ? getSectionById(report, "yearly-bank-scenarios") : undefined
  const glossary = useMemo(() => (report ? extractGlossaryTerms(report) : []), [report])

  if (loading) {
    return (
      <div className="flex min-h-[50vh] items-center text-sm text-muted-foreground">
        Loading report from <code className="mx-1.5 font-mono">data/</code>…
      </div>
    )
  }

  if (!report) {
    return (
      <div className="flex min-h-[60vh] flex-col items-start justify-center space-y-4">
        <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
          Load a financial analysis report
        </h1>
        <p className="max-w-md text-sm text-muted-foreground">
          Select an Alcazar <code className="font-mono">AnalysisReport</code> JSON file to inspect
          metrics, ratios, risk tables, and historical filings.
        </p>
        <Button type="button" size="sm" onClick={openFilePicker}>
          <Upload className="size-4" />
          Load JSON
        </Button>
      </div>
    )
  }

  const isCorporate = report.company?.industry === "corporate"
  const corpSection = isCorporate ? getSectionById(report, "yearly-corporate") : undefined

  interface KpiCard {
    label: string
    labelTooltip?: string
    value: string
    delta: string
    sub: string
    subTooltip?: string
  }

  const kpis: KpiCard[] = isCorporate && corpSection
    ? [
        {
          label: `FY${latestAnnual?.period_label ?? "Latest"} Revenue`,
          value: extractRowValue(corpSection, "Revenue"),
          delta: extractRowDelta(corpSection, "Revenue"),
          sub: `Profit ${extractRowValue(latestAnnual, "Group profit")}`,
        },
        {
          label: "Operating Margin",
          labelTooltip:
            "Share of revenue left after paying all day-to-day operating costs. Higher means a more efficient business.",
          value: extractRowValue(corpSection, "Operating margin"),
          delta: extractRowDelta(corpSection, "Operating margin"),
          sub: `ROE ${extractRowValue(latestAnnual, "Return on equity")}`,
        },
        {
          label: "Free Cash Flow",
          labelTooltip:
            "Cash left over from operations after paying for equipment and upkeep (CapEx).",
          value: extractRowValue(corpSection, "Free cash flow"),
          delta: extractRowDelta(corpSection, "Free cash flow"),
          sub: `ROIC ${extractRowValue(corpSection, "ROIC")}`,
        },
        {
          label: "Net Debt / EBITDA",
          labelTooltip:
            "How many years of operating cash profit it would take to pay off total debt (minus cash). Lower is safer.",
          value: extractRowValue(corpSection, "Net debt / EBITDA"),
          delta: "",
          sub: `P/E ${extractRowValue(latestAnnual, "Price / earnings")}`,
        },
      ].filter((kpi) => kpi.value !== "—")
    : [
    {
      label: `FY${latestAnnual?.period_label ?? "Latest"} Profit`,
      value: extractRowValue(latestAnnual, "Group profit"),
      delta: extractRowDelta(latestAnnual, "Group profit"),
      sub: `EPS ${extractRowValue(latestAnnual, "Earnings per share")}`,
      subTooltip:
        "The bank's net profit divided by its total number of shares—showing how much profit each single share earned.",
    },
    {
      label: `FY${latestAnnual?.period_label ?? "Latest"} Return on Equity`,
      labelTooltip:
        "Measures how efficiently the bank generates profit from shareholders' equity. Higher means more profit per rupiah of equity.",
      value: extractRowValue(latestAnnual, "Return on equity"),
      delta: extractRowDelta(latestAnnual, "Return on equity"),
      sub: `ROA ${extractRowValue(latestAnnual, "Return on assets")}`,
      subTooltip:
        "Return on Assets—measures how efficiently the bank uses its total assets (loans, investments, cash) to generate profit. For banks, above 1% is generally solid and 2%+ is strong.",
    },
    ...(bankRisk
      ? [
          {
            label: `FY${latestAnnual?.period_label ?? "Latest"} CET1 Ratio`,
            labelTooltip:
              "The bank's strictest safety cushion—its core shareholder money compared to the riskiness of its loans and investments. Higher means safer (above 10–12% is generally well-capitalized).",
            value: extractRowValue(bankRisk, "CET1 ratio"),
            delta: "",
            sub: `CAR ${extractRowValue(latestAnnual, "Capital adequacy")}`,
            subTooltip:
              "Regulators and investors use it to verify that a bank holds enough total capital relative to the riskiness of its loans and investments. Above 15% is generally a strong buffer.",
          },
        ]
      : [
          {
            label: `FY${latestAnnual?.period_label ?? "Latest"} Price / Earnings`,
            value: extractRowValue(latestAnnual, "Price / earnings"),
            delta: extractRowDelta(latestAnnual, "Price / earnings"),
            sub: `P/BV ${extractRowValue(latestAnnual, "Price / book value")}`,
          },
        ]),
    ...(latestInterim
      ? [
          {
            label: latestInterim.period_label ?? "Interim",
            value: extractRowValue(latestInterim, "Group"),
            delta: extractRowDelta(latestInterim, "Group"),
            sub: "Interim update",
          },
        ]
      : [
          {
            label: "Total Return",
            value: extractRowValue(latestAnnual, "Return including cash dividends"),
            delta: "",
            sub: `Yield ${extractRowValue(latestAnnual, "Dividend yield")}`,
          },
        ]),
  ].filter((kpi) => kpi.value !== "—")

  const hasProfitOrRoe = chartSeries.some((p) => p.profit !== null || p.roe !== null)
  const hasCompounding = chartSeries.some(
    (p) => p.profitIndex !== null || p.returnIndex !== null,
  )

  return (
    <div className="space-y-12 min-w-0">
      {/* 1. OVERVIEW */}
      <section id="sec-overview" className="scroll-mt-14 space-y-8 border-b pb-10 min-w-0">
        <header className="space-y-2">
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
              {ticker ? `${ticker} Financial Report` : "Financial Report"}
            </h1>
            {report.company?.industry || report.company?.currency ? (
              <span className="rounded-md bg-muted px-2 py-0.5 font-mono text-xs text-muted-foreground">
                {[report.company.industry, report.company.currency].filter(Boolean).join(" · ")}
              </span>
            ) : null}
          </div>
          <p className="max-w-2xl text-sm text-muted-foreground leading-relaxed">
            {getSectionById(report, "yearly-cover")?.paragraphs?.[0]?.text ??
              "Annual performance, valuation, risk, and historical records."}
          </p>
        </header>

        {kpis.length > 0 ? (
          <div className="grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-4" aria-label="Key metrics">
            {kpis.map((kpi) => {
              const isNeg = kpi.delta.startsWith("-")
              return (
                <div key={kpi.label} className="min-w-0">
                  <div className="flex items-center gap-1 min-w-0">
                    <p className="truncate text-xs text-muted-foreground">{kpi.label}</p>
                    {kpi.labelTooltip ? <KpiInfoTooltip text={kpi.labelTooltip} /> : null}
                  </div>
                  <p className="mt-1 font-mono text-xl font-semibold tabular-nums sm:text-2xl">
                    {kpi.value}
                  </p>
                  <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs">
                    {kpi.delta ? (
                      <span
                        className={`font-mono font-medium ${
                          isNeg
                            ? "text-rose-700 dark:text-rose-400"
                            : "text-teal-700 dark:text-teal-400"
                        }`}
                      >
                        {kpi.delta}
                      </span>
                    ) : null}
                    <span className="inline-flex items-center gap-1 min-w-0 text-muted-foreground">
                      <span className="truncate">{kpi.sub}</span>
                      {kpi.subTooltip ? <KpiInfoTooltip text={kpi.subTooltip} /> : null}
                    </span>
                  </div>
                </div>
              )
            })}
          </div>
        ) : null}

        {hasProfitOrRoe || hasCompounding ? (
          <div className="space-y-3 min-w-0">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-1">
                {hasProfitOrRoe ? (
                  <Button
                    type="button"
                    size="xs"
                    variant={chartMode === "profit" ? "secondary" : "ghost"}
                    onClick={() => setChartMode("profit")}
                  >
                    Profit &amp; ROE
                  </Button>
                ) : null}
                {hasCompounding ? (
                  <Button
                    type="button"
                    size="xs"
                    variant={chartMode === "compounding" ? "secondary" : "ghost"}
                    onClick={() => setChartMode("compounding")}
                  >
                    Compounding
                  </Button>
                ) : null}
              </div>

              <div className="flex items-center gap-1">
                {(["5", "10", "all"] as const).map((s) => (
                  <Button
                    key={s}
                    type="button"
                    size="xs"
                    variant={span === s ? "secondary" : "ghost"}
                    onClick={() => setSpan(s)}
                  >
                    {s === "all" ? "All" : `${s}Y`}
                  </Button>
                ))}
              </div>
            </div>

            {chartMode === "profit" && hasProfitOrRoe ? (
              <p className="text-xs text-muted-foreground">
                Bars show annual net profit (left axis); line shows Return on Equity (right axis).
              </p>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <p>
                  Tracks long-term compounding—how many times profit and total return have multiplied
                  since the start.
                </p>
                <span className="font-mono text-[11px]">
                  100 = start · 1,000 = 10× · 10,000 = 100×
                </span>
              </div>
            )}

            {chartMode === "profit" && hasProfitOrRoe ? (
              <ProfitRoeChart data={chartSeries} currency={report.company?.currency ?? "IDR"} />
            ) : hasCompounding ? (
              <IndexedCompoundingChart data={chartSeries} />
            ) : null}
          </div>
        ) : null}

        {latestInterim ? (() => {
          const interimHeading = splitTitleSummary(latestInterim.title)
          return (
            <div className="space-y-3 min-w-0">
              <div className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-1.5">
                  <h3 className="text-sm font-medium text-foreground/90">
                    {interimHeading.label || "Latest interim"}
                  </h3>
                </div>
                {(latestInterim.tables?.length ?? 0) > 0 ? (
                  <Button
                    type="button"
                    size="xs"
                    variant="ghost"
                    onClick={() => setShowInterimTables((prev) => !prev)}
                  >
                    <Table2 className="size-3.5" />
                    <span>Tables</span>
                    <ChevronDown
                      className={`size-3 transition-transform ${
                        showInterimTables ? "rotate-180" : ""
                      }`}
                    />
                  </Button>
                ) : null}
              </div>
              {latestInterim.paragraphs?.[0]?.text ? (
                <p className="max-w-3xl text-[13px] leading-relaxed text-muted-foreground">
                  {latestInterim.paragraphs[0].text}
                </p>
              ) : null}
              {showInterimTables ? (
                <div className="space-y-8 pt-2">
                  {(latestInterim.tables ?? []).map((table, idx) => (
                    <FinancialTable key={table.title ?? idx} table={table} />
                  ))}
                </div>
              ) : null}
            </div>
          )
        })() : null}
      </section>

      {/* 2. RATIOS */}
      <RatiosSection series={basePoint ? allSeries.filter((p) => p.year >= basePoint.year) : allSeries} />

      {/* 3. RISK & STRESS */}
      <RiskStressSection
        ticker={ticker}
        currencySymbol={report.company?.currency_symbol ?? "Rp"}
        bankRisk={bankRisk}
        scenarioSection={scenarioSection}
        scenarios={report.bank_scenarios}
      />

      {/* 4. HISTORY */}
      <HistorySection annualSections={annualSections} />

      {/* 5. GLOSSARY & AUDIT */}
      <AuditSection
        glossary={glossary}
        rules={report.rules ?? []}
        sources={report.sources ?? {}}
      />
    </div>
  )
}
