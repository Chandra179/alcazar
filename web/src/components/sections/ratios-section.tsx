import { BarChart3, Table2 } from "lucide-react"
import { useState } from "react"

import { AnnualReturnsChart } from "@/components/charts/annual-returns-chart"
import { RatiosSmallMultiples } from "@/components/charts/ratios-small-multiples"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { computeMedian } from "@/lib/report-data"
import type { AnnualPoint } from "@/types/analysis"

interface RatiosSectionProps {
  series: AnnualPoint[]
}

const METRICS: {
  key: keyof AnnualPoint
  label: string
  unit: string
  decimals: number
}[] = [
  { key: "roe", label: "Return on equity (ROE)", unit: "%", decimals: 2 },
  { key: "roa", label: "Return on assets (ROA)", unit: "%", decimals: 2 },
  { key: "nim", label: "Net interest margin (NIM)", unit: "%", decimals: 2 },
  { key: "pe", label: "Price / earnings (P/E)", unit: "×", decimals: 1 },
  { key: "pbv", label: "Price / book value (P/BV)", unit: "×", decimals: 2 },
  { key: "divYield", label: "Dividend yield", unit: "%", decimals: 2 },
  { key: "totalReturn", label: "Annual total return", unit: "%", decimals: 2 },
]

export function RatiosSection({ series }: RatiosSectionProps) {
  const [view, setView] = useState<"table" | "charts">("table")
  const [showMedian, setShowMedian] = useState(true)

  const rows = METRICS.map((m) => {
    const vals = series
      .map((p) => ({ label: p.label, v: p[m.key] as number | null }))
      .filter((x): x is { label: string; v: number } => x.v !== null)
    if (vals.length === 0) return null
    const latest = vals[vals.length - 1]
    const median = computeMedian(vals.map((x) => x.v))
    const min = Math.min(...vals.map((x) => x.v))
    const max = Math.max(...vals.map((x) => x.v))
    return { ...m, latest, median, min, max }
  }).filter((r): r is NonNullable<typeof r> => r !== null)

  if (rows.length === 0) return null

  const hasReturns = series.some((p) => p.totalReturn !== null)

  return (
    <section id="sec-ratios" className="scroll-mt-14 space-y-5 border-b pb-10 min-w-0">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-semibold tracking-tight">Ratios &amp; Valuation</h2>

        <div className="flex items-center gap-1.5">
          {view === "charts" ? (
            <Button
              type="button"
              size="xs"
              variant={showMedian ? "secondary" : "ghost"}
              onClick={() => setShowMedian((prev) => !prev)}
            >
              Median {showMedian ? "On" : "Off"}
            </Button>
          ) : null}
          <Button
            type="button"
            size="xs"
            variant={view === "table" ? "secondary" : "ghost"}
            onClick={() => setView("table")}
            title="Table view"
          >
            <Table2 className="size-3.5" />
            Table
          </Button>
          <Button
            type="button"
            size="xs"
            variant={view === "charts" ? "secondary" : "ghost"}
            onClick={() => setView("charts")}
            title="Chart view"
          >
            <BarChart3 className="size-3.5" />
            Charts
          </Button>
        </div>
      </div>

      {view === "table" ? (
        <div className="min-w-0">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="sticky left-0 z-10 bg-background pr-4">Ratio</TableHead>
                <TableHead className="text-right">Latest</TableHead>
                <TableHead className="text-right">Median</TableHead>
                <TableHead className="text-right">Range</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r) => (
                <TableRow key={r.key}>
                  <TableCell className="sticky left-0 z-10 bg-background pr-4 font-medium whitespace-nowrap">
                    {r.label}
                  </TableCell>
                  <TableCell className="text-right font-mono font-semibold tabular-nums whitespace-nowrap">
                    {r.latest.v.toFixed(r.decimals)}
                    {r.unit}
                    <span className="ml-1.5 text-xs font-normal text-muted-foreground">
                      ({r.latest.label})
                    </span>
                  </TableCell>
                  <TableCell className="text-right font-mono text-muted-foreground tabular-nums whitespace-nowrap">
                    {r.median !== null ? `${r.median.toFixed(r.decimals)}${r.unit}` : "—"}
                  </TableCell>
                  <TableCell className="text-right font-mono text-muted-foreground tabular-nums whitespace-nowrap">
                    {r.min.toFixed(r.decimals)}–{r.max.toFixed(r.decimals)}
                    {r.unit}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        <div className="space-y-6">
          <RatiosSmallMultiples data={series} showMedian={showMedian} />
          {hasReturns ? (
            <div className="min-w-0 space-y-2 pt-2">
              <h3 className="text-sm font-medium">Annual Total Return (%)</h3>
              <AnnualReturnsChart data={series} />
            </div>
          ) : null}
        </div>
      )}
    </section>
  )
}
