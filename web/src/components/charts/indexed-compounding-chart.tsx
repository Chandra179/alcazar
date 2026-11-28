import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

import type { AnnualPoint } from "@/types/analysis"

interface IndexedCompoundingChartProps {
  data: AnnualPoint[]
}

export function IndexedCompoundingChart({ data }: IndexedCompoundingChartProps) {
  const filtered = data.filter(
    (d) => (d.profitIndex !== null && d.profitIndex > 0) || (d.returnIndex !== null && d.returnIndex > 0),
  )

  return (
    <div className="h-[320px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={filtered} margin={{ top: 10, right: 12, left: 4, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" vertical={false} className="stroke-border" />
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            minTickGap={16}
          />
          <YAxis
            scale="log"
            domain={["auto", "auto"]}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v: number) => `${Math.round(v)}`}
            width={52}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--popover)",
              borderColor: "var(--border)",
              borderRadius: "8px",
              fontSize: "12px",
            }}
            formatter={(value, name) => {
              if (value === null || value === undefined) return ["—", String(name)]
              return [`${Number(value).toLocaleString(undefined, { maximumFractionDigits: 0 })}x (base 100)`, String(name)]
            }}
          />
          <Line
            type="monotone"
            dataKey="returnIndex"
            name="Cumulative total return"
            stroke="#0f766e"
            strokeWidth={2.25}
            dot={false}
            activeDot={{ r: 4 }}
            connectNulls
          />
          <Line
            type="monotone"
            dataKey="profitIndex"
            name="Group profit index"
            stroke="#334155"
            strokeWidth={2}
            strokeDasharray="4 3"
            dot={false}
            activeDot={{ r: 4 }}
            connectNulls
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
