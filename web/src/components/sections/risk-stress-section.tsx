import { BarChart3, ExternalLink, FileText, HelpCircle, RotateCcw } from "lucide-react"
import { useEffect, useMemo, useState } from "react"

import { CasaPeersChart, type CasaPeerPoint } from "@/components/charts/casa-peers-chart"
import { ScenarioValuationChart } from "@/components/charts/scenario-valuation-chart"
import { FinancialTable } from "@/components/financial-table"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import {
  buildScenarioOutputTables,
  evaluateBankScenario,
  extractScenarioInputs,
} from "@/lib/bank-scenario-sim"
import { getPlainTermTooltip } from "@/lib/glossary-terms"
import { getSectionSources, parseMetricNumber } from "@/lib/report-data"
import type { BankScenarioCase, BankScenarios, ReportSection } from "@/types/analysis"

interface RiskStressSectionProps {
  ticker: string
  currencySymbol?: string
  bankRisk?: ReportSection
  scenarioSection?: ReportSection
  scenarios?: BankScenarios
}

function cloneDefaultCases(scenarios?: BankScenarios): BankScenarioCase[] {
  if (!scenarios?.results?.length) return []
  return scenarios.results
    .map((r) => r.assumptions ?? r.case)
    .filter((c): c is BankScenarioCase => Boolean(c))
    .map((c) => ({
      ...c,
      normalized_roe: c.normalized_roe ?? c.roe ?? 0,
      one_year_stress: c.one_year_stress ? { ...c.one_year_stress } : undefined,
    }))
}

function HeaderTerm({ text }: { text: string }) {
  const tip = getPlainTermTooltip(text)
  if (!tip) return <>{text}</>
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          tabIndex={0}
          className="cursor-help underline decoration-dotted decoration-muted-foreground/60 underline-offset-4 focus-visible:outline-none"
        >
          {text}
        </span>
      </TooltipTrigger>
      <TooltipContent>{tip}</TooltipContent>
    </Tooltip>
  )
}

function InlinePctInput({
  value,
  step = 0.5,
  onChange,
  ariaLabel,
}: {
  value: number
  step?: number
  onChange: (pct: number) => void
  ariaLabel: string
}) {
  return (
    <span className="inline-flex items-baseline">
      <input
        type="number"
        step={step}
        min={0}
        max={100}
        aria-label={ariaLabel}
        value={Number((value * 100).toFixed(1))}
        onChange={(e) => {
          const v = parseFloat(e.target.value)
          if (!Number.isNaN(v)) onChange(v)
        }}
        className="w-11 border-b border-dashed border-muted-foreground/40 bg-transparent text-right font-mono text-[13px] tabular-nums hover:border-foreground focus:border-solid focus:border-foreground focus:outline-none"
      />
      <span className="font-mono text-[13px]">%</span>
    </span>
  )
}

export function RiskStressSection({
  ticker,
  currencySymbol = "Rp",
  bankRisk,
  scenarioSection,
  scenarios,
}: RiskStressSectionProps) {
  const [showCharts, setShowCharts] = useState(false)

  const defaultCases = useMemo(() => cloneDefaultCases(scenarios), [scenarios])
  const [customCases, setCustomCases] = useState<BankScenarioCase[]>(defaultCases)

  useEffect(() => {
    setCustomCases(defaultCases)
  }, [defaultCases])

  const simInputs = useMemo(() => extractScenarioInputs(scenarios), [scenarios])
  const canSimulate = Boolean(simInputs && customCases.length > 0)

  const isModified = useMemo(
    () => canSimulate && JSON.stringify(customCases) !== JSON.stringify(defaultCases),
    [canSimulate, customCases, defaultCases],
  )

  const liveResults = useMemo(() => {
    if (!canSimulate || !simInputs) return scenarios?.results ?? []
    return customCases.map((c) => evaluateBankScenario(c, simInputs))
  }, [canSimulate, customCases, scenarios?.results, simInputs])

  const outputTables = useMemo(() => {
    if (canSimulate) return buildScenarioOutputTables(liveResults, scenarios?.results, currencySymbol)
    return scenarioSection?.tables ?? []
  }, [canSimulate, currencySymbol, liveResults, scenarioSection?.tables, scenarios?.results])

  const casaPeers = useMemo<CasaPeerPoint[]>(() => {
    const peerTable = bankRisk?.tables?.find((t) =>
      t.title?.toLowerCase().includes("casa"),
    )
    if (!peerTable) return []
    return peerTable.rows
      .map((row) => ({
        bank: row[0]?.text || row[0]?.links?.[0]?.text || "Peer",
        share: parseMetricNumber(row[1]?.text ?? "") ?? 0,
        scope: row[2]?.text ?? "",
      }))
      .filter((item) => item.share > 0)
  }, [bankRisk])

  const sourceLinks = useMemo(() => {
    const seen = new Set<string>()
    const links: { text: string; url: string }[] = []
    for (const sec of [scenarioSection, bankRisk]) {
      if (!sec) continue
      for (const cell of getSectionSources(sec)) {
        for (const link of cell.links ?? []) {
          if (!seen.has(link.url)) {
            seen.add(link.url)
            links.push(link)
          }
        }
      }
    }
    return links
  }, [scenarioSection, bankRisk])

  if (!bankRisk && !scenarioSection && !scenarios?.results?.length) {
    return null
  }

  const updateCaseField = (idx: number, key: keyof BankScenarioCase, pctValue: number) => {
    const clamped = Math.min(1, Math.max(0, pctValue / 100))
    setCustomCases((prev) =>
      prev.map((c, i) => (i === idx ? { ...c, [key]: clamped } : c)),
    )
  }

  const bankRiskTables = bankRisk?.tables ?? []

  return (
    <section id="sec-risk" className="scroll-mt-14 space-y-5 border-b pb-10 min-w-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <div className="flex flex-wrap items-center gap-2.5">
            <h2 className="text-xl font-semibold tracking-tight">Risk &amp; Stress</h2>
            {scenarios?.period ? (
              <span className="font-mono text-xs text-muted-foreground">{scenarios.period}</span>
            ) : null}
            <Popover>
              <PopoverTrigger asChild>
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  className="text-muted-foreground hover:text-foreground"
                >
                  <HelpCircle className="size-3.5" />
                  How it works
                </Button>
              </PopoverTrigger>
              <PopoverContent>
                <h3 className="mb-2 text-[13px] font-semibold text-foreground">
                  How to read Risk &amp; Stress
                </h3>
                <ul className="space-y-1.5 pl-4 text-xs leading-relaxed text-muted-foreground list-disc">
                  <li>
                    <strong className="text-foreground">What it is:</strong> Estimates what one
                    share could be worth and checks if the bank&apos;s safety cushion stays healthy
                    across three cases.
                  </li>
                  <li>
                    <strong className="text-foreground">Why we use it:</strong> Good years can hide
                    risk. Stress testing shows how much trouble the bank can handle in a bad year.
                  </li>
                  <li>
                    <strong className="text-foreground">How to read it:</strong> Compare{" "}
                    <strong className="text-foreground">Downside</strong> (bad year),{" "}
                    <strong className="text-foreground">Base</strong> (normal year), and{" "}
                    <strong className="text-foreground">Favorable</strong> (strong year).
                  </li>
                </ul>
              </PopoverContent>
            </Popover>
          </div>
          <p className="text-[13px] text-muted-foreground">
            Estimated per-share value and capital resilience across downside, base, and favorable cases.
          </p>
        </div>

        <div className="flex items-center gap-1.5">
          {casaPeers.length > 0 || liveResults.length ? (
            <Button
              type="button"
              size="xs"
              variant={showCharts ? "secondary" : "ghost"}
              onClick={() => setShowCharts((prev) => !prev)}
            >
              <BarChart3 className="size-3.5" />
              Charts
            </Button>
          ) : null}
          {sourceLinks.length > 0 ? (
            <Sheet>
              <SheetTrigger asChild>
                <Button type="button" size="xs" variant="ghost">
                  <FileText className="size-3.5" />
                  Sources ({sourceLinks.length})
                </Button>
              </SheetTrigger>
              <SheetContent>
                <SheetHeader>
                  <SheetTitle>Sources</SheetTitle>
                  <SheetDescription>
                    Primary filings and disclosures for Risk &amp; Stress.
                  </SheetDescription>
                </SheetHeader>
                <ul className="flex-1 divide-y overflow-y-auto px-5 py-3">
                  {sourceLinks.map((link) => (
                    <li key={link.url} className="py-2.5">
                      <a
                        href={link.url}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-start gap-1.5 text-[13px] leading-snug text-teal-700 hover:underline dark:text-teal-400"
                      >
                        <span>{link.text}</span>
                        <ExternalLink className="mt-0.5 size-3 shrink-0 opacity-70" />
                      </a>
                    </li>
                  ))}
                </ul>
              </SheetContent>
            </Sheet>
          ) : null}
        </div>
      </div>

      {showCharts ? (
        <div className="grid gap-6 border-b pb-6 lg:grid-cols-2">
          {liveResults.length ? (
            <div className="min-w-0 space-y-2">
              <h3 className="text-sm font-medium">Per-Share Valuation by Scenario</h3>
              <ScenarioValuationChart results={liveResults} currencySymbol={currencySymbol} />
            </div>
          ) : null}
          {casaPeers.length > 0 ? (
            <div className="min-w-0 space-y-2">
              <h3 className="text-sm font-medium">Peer CASA Funding Share (%)</h3>
              <CasaPeersChart data={casaPeers} primaryTicker={ticker} />
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="space-y-9">
        {canSimulate ? (
          <div className="space-y-2.5 min-w-0">
            <div className="flex items-center justify-between gap-2">
              <h3 className="text-sm font-medium text-foreground/90">Valuation assumptions</h3>
              {isModified ? (
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  onClick={() => setCustomCases(defaultCases)}
                  className="text-muted-foreground hover:text-foreground"
                >
                  <RotateCcw className="size-3" />
                  Reset
                </Button>
              ) : null}
            </div>
            <div className="overflow-x-auto">
              <Table className="text-[13px]">
                <TableHeader>
                  <TableRow className="border-b hover:bg-transparent">
                    <TableHead className="h-9 px-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      Case
                    </TableHead>
                    <TableHead className="h-9 px-2 text-right text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      <HeaderTerm text="Earnings / RWA growth" />
                    </TableHead>
                    <TableHead className="h-9 px-2 text-right text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      <HeaderTerm text="Cost of equity / terminal growth" />
                    </TableHead>
                    <TableHead className="h-9 px-2 text-right text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      Payout target
                    </TableHead>
                    <TableHead className="h-9 px-2 text-right text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      Value at +1 pp cost
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {customCases.map((c, idx) => {
                    const res = liveResults[idx]
                    const higherRate =
                      res?.dividend_value_at_cost_plus_1pp ?? res?.dividend_value_higher_rate ?? 0
                    const baseRes = scenarios?.results?.[idx]
                    const baseHigherRate =
                      baseRes?.dividend_value_at_cost_plus_1pp ??
                      baseRes?.dividend_value_higher_rate ??
                      higherRate
                    const higherRateChanged =
                      isModified && Math.round(higherRate) !== Math.round(baseHigherRate)
                    return (
                      <TableRow key={c.name} className="border-b/60 hover:bg-muted/30">
                        <TableCell className="px-2 py-2.5 font-normal">{c.name}</TableCell>
                        <TableCell className="px-2 py-2.5 text-right font-mono tabular-nums">
                          <span className="inline-flex items-center gap-1">
                            <InlinePctInput
                              value={c.earnings_growth}
                              onChange={(v) => updateCaseField(idx, "earnings_growth", v)}
                              ariaLabel={`${c.name} earnings growth`}
                            />
                            <span>/</span>
                            <InlinePctInput
                              value={c.rwa_growth}
                              onChange={(v) => updateCaseField(idx, "rwa_growth", v)}
                              ariaLabel={`${c.name} RWA growth`}
                            />
                          </span>
                        </TableCell>
                        <TableCell className="px-2 py-2.5 text-right font-mono tabular-nums">
                          <span className="inline-flex items-center gap-1">
                            <InlinePctInput
                              value={c.cost_of_equity}
                              onChange={(v) => updateCaseField(idx, "cost_of_equity", v)}
                              ariaLabel={`${c.name} cost of equity`}
                            />
                            <span>/</span>
                            <InlinePctInput
                              value={c.terminal_growth}
                              onChange={(v) => updateCaseField(idx, "terminal_growth", v)}
                              ariaLabel={`${c.name} terminal growth`}
                            />
                          </span>
                        </TableCell>
                        <TableCell className="px-2 py-2.5 text-right font-mono tabular-nums">
                          <InlinePctInput
                            value={c.payout}
                            step={1}
                            onChange={(v) => updateCaseField(idx, "payout", v)}
                            ariaLabel={`${c.name} payout target`}
                          />
                        </TableCell>
                        <td
                          className={`px-2 py-2.5 text-right font-mono tabular-nums ${
                            higherRateChanged ? "font-medium text-teal-700 dark:text-teal-400" : ""
                          }`}
                        >
                          {currencySymbol}
                          {Math.round(higherRate)}
                        </td>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          </div>
        ) : null}

        {outputTables.map((table, idx) => (
          <FinancialTable key={table.title ?? idx} table={table} />
        ))}

        {bankRiskTables.map((table, idx) => (
          <FinancialTable key={table.title ?? `risk-${idx}`} table={table} />
        ))}
      </div>
    </section>
  )
}
