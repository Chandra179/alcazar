import { BarChart3, ExternalLink, FileText, HelpCircle } from "lucide-react"
import { useMemo, useState } from "react"

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
import { getSectionSources, parseMetricNumber } from "@/lib/report-data"
import type { BankScenarios, ReportSection } from "@/types/analysis"

interface RiskStressSectionProps {
  ticker: string
  bankRisk?: ReportSection
  scenarioSection?: ReportSection
  scenarios?: BankScenarios
}

export function RiskStressSection({
  ticker,
  bankRisk,
  scenarioSection,
  scenarios,
}: RiskStressSectionProps) {
  const [showCharts, setShowCharts] = useState(false)

  const casaPeers = useMemo<CasaPeerPoint[]>(() => {
    const peerTable = bankRisk?.tables?.[1]
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

  const allTables = [
    ...(scenarioSection?.tables ?? []),
    ...(bankRisk?.tables ?? []),
  ]

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
          {casaPeers.length > 0 || scenarios?.results?.length ? (
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
          {scenarios?.results?.length ? (
            <div className="min-w-0 space-y-2">
              <h3 className="text-sm font-medium">Per-Share Valuation by Scenario</h3>
              <ScenarioValuationChart results={scenarios.results} />
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
        {allTables.map((table, idx) => (
          <FinancialTable key={table.title ?? idx} table={table} />
        ))}
      </div>
    </section>
  )
}
