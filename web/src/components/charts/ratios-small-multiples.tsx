import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

import { computeMedian } from "@/lib/report-data"
import type { AnnualPoint } from "@/types/analysis"

interface RatioPanelConfig {
  key: keyof AnnualPoint
  title: string
  unit: string
  decimals: number
}

const PANELS: RatioPanelConfig[] = [
  { key: "roe", title: "Return on equity (ROE)", unit: "%", decimals: 1 },
  { key: "roa", title: "Return on assets (ROA)", unit: "%", decimals: 2 },
  { key: "pe", title: "Price / earnings (P/E)", unit: "×", decimals: 1 },
  { key: "pbv", title: "Price / book value (P/BV)", unit: "×", decimals: 2 },
  { key: "divYield", title: "Dividend yield", unit: "%", decimals: 2 },
  { key: "nim", title: "Net interest margin (NIM)", unit: "%", decimals: 2 },
]

interface RatiosSmallMultiplesProps {
  data: AnnualPoint[]
  showMedian: boolean
}

export function RatiosSmallMultiples({ data, showMedian }: RatiosSmallMultiplesProps) {
  const activePanels = PANELS.filter((panel) => data.some((d) => d[panel.key] !== null))
  if (activePanels.length === 0) return null

  return (
    <div className="grid gap-x-8 gap-y-6 sm:grid-cols-2 lg:grid-cols-3">
      {activePanels.map((panel) => {
        const values = data.map((d) => d[panel.key] as number | null)
        const median = computeMedian(values)
        const latest = [...data].reverse().find((d) => d[panel.key] !== null)
        const latestVal = latest ? (latest[panel.key] as number) : null

        return (
          <div key={panel.key} className="space-y-2 border-b pb-4">
            <div className="flex items-baseline justify-between gap-2">
              <h3 className="text-sm font-medium">{panel.title}</h3>
              <span className="font-mono text-sm font-semibold tabular-nums text-teal-700 dark:text-teal-400">
                {latestVal !== null ? `${latestVal.toFixed(panel.decimals)}${panel.unit}` : "—"}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {median !== null
                ? `Median: ${median.toFixed(panel.decimals)}${panel.unit}`
                : "No median"}
              {latest ? ` · Latest (${latest.label})` : ""}
            </p>
            <div className="h-[150px] w-full">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={data} margin={{ top: 6, right: 8, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} className="stroke-border" />
                  <XAxis
                    dataKey="label"
                    tick={{ fontSize: 10 }}
                    tickLine={false}
                    axisLine={false}
                    minTickGap={22}
                  />
                  <YAxis
                    tick={{ fontSize: 10 }}
                    tickLine={false}
                    axisLine={false}
                    width={34}
                    tickFormatter={(v: number) => `${v}${panel.unit}`}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "var(--popover)",
                      borderColor: "var(--border)",
                      borderRadius: "8px",
                      fontSize: "12px",
                    }}
                    formatter={(val) =>
                      val !== null && val !== undefined
                        ? [`${Number(val).toFixed(panel.decimals)}${panel.unit}`, panel.title]
                        : ["—", panel.title]
                    }
                  />
                  {showMedian && median !== null ? (
                    <ReferenceLine
                      y={median}
                      stroke="#94a3b8"
                      strokeDasharray="4 4"
                      strokeWidth={1.25}
                    />
                  ) : null}
                  <Line
                    type="monotone"
                    dataKey={panel.key}
                    stroke="#0f766e"
                    strokeWidth={2}
                    dot={false}
                    activeDot={{ r: 4 }}
                    connectNulls
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>
        )
      })}
    </div>
  )
}
