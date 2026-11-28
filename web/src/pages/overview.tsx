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

  const kpis = [
    {
      label: `FY${latestAnnual?.period_label ?? "Latest"} Profit`,
      value: extractRowValue(latestAnnual, "Group profit"),
      delta: extractRowDelta(latestAnnual, "Group profit"),
      sub: `EPS ${extractRowValue(latestAnnual, "Earnings per share")}`,
    },
    {
      label: "Return on Equity",
      value: extractRowValue(latestAnnual, "Return on equity"),
      delta: extractRowDelta(latestAnnual, "Return on equity"),
      sub: `ROA ${extractRowValue(latestAnnual, "Return on assets")}`,
    },
    ...(bankRisk
      ? [
          {
            label: "CET1 Ratio",
            value: extractRowValue(bankRisk, "CET1 ratio"),
            delta: "",
            sub: `CAR ${extractRowValue(latestAnnual, "Capital adequacy")}`,
          },
        ]
      : [
          {
            label: "Price / Earnings",
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
          <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
            {ticker ? `${ticker} Financial Report` : "Financial Report"}
          </h1>
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
                  <p className="truncate text-xs text-muted-foreground">{kpi.label}</p>
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
                    <span className="truncate text-muted-foreground">{kpi.sub}</span>
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
              <ProfitRoeChart data={chartSeries} />
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
                  {interimHeading.summary ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          aria-label={interimHeading.summary}
                          className="inline-flex items-center text-muted-foreground hover:text-foreground focus-visible:outline-none"
                        >
                          <Info className="size-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>{interimHeading.summary}</TooltipContent>
                    </Tooltip>
                  ) : null}
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
