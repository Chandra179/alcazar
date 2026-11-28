import { BarChart3, ChevronDown, FileText } from "lucide-react"
import { useMemo, useState } from "react"

import { CasaPeersChart, type CasaPeerPoint } from "@/components/charts/casa-peers-chart"
import { ScenarioValuationChart } from "@/components/charts/scenario-valuation-chart"
import { FinancialTable } from "@/components/financial-table"
import { SectionTakeaways } from "@/components/section-takeaways"
import { Button } from "@/components/ui/button"
import { parseMetricNumber } from "@/lib/report-data"
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
  const [showNotes, setShowNotes] = useState(false)

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

  if (!bankRisk && !scenarioSection && !scenarios?.results?.length) {
    return null
  }

  const allTables = [
    ...(scenarioSection?.tables ?? []),
    ...(bankRisk?.tables ?? []),
  ]

  return (
    <section id="sec-risk" className="scroll-mt-14 space-y-5 border-b pb-10 min-w-0">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-baseline gap-2.5">
          <h2 className="text-lg font-semibold tracking-tight">Risk &amp; Stress</h2>
          {scenarios?.period ? (
            <span className="font-mono text-xs text-muted-foreground">{scenarios.period}</span>
          ) : null}
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
          {bankRisk || scenarioSection ? (
            <Button
              type="button"
              size="xs"
              variant={showNotes ? "secondary" : "ghost"}
              onClick={() => setShowNotes((prev) => !prev)}
            >
              <FileText className="size-3.5" />
              Notes
              <ChevronDown
                className={`size-3 transition-transform ${showNotes ? "rotate-180" : ""}`}
              />
            </Button>
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

      <div className="space-y-6">
        {allTables.map((table, idx) => (
          <FinancialTable key={table.title ?? idx} table={table} compact />
        ))}
      </div>

      {showNotes ? (
        <div className="space-y-4 pt-2">
          {scenarioSection ? <SectionTakeaways section={scenarioSection} /> : null}
          {bankRisk ? <SectionTakeaways section={bankRisk} /> : null}
        </div>
      ) : null}
    </section>
  )
}
