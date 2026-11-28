import {
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

import type { AnnualPoint } from "@/types/analysis"

interface ProfitRoeChartProps {
  data: AnnualPoint[]
  currency?: string
}

export function ProfitRoeChart({ data, currency = "IDR" }: ProfitRoeChartProps) {
  return (
    <div className="h-[320px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={data} margin={{ top: 10, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" vertical={false} className="stroke-border" />
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            minTickGap={16}
          />
          <YAxis
            yAxisId="profit"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v: number) => `${v}T`}
            width={44}
          />
          <YAxis
            yAxisId="roe"
            orientation="right"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v: number) => `${v}%`}
            width={42}
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
              const num = Number(value)
              if (name === "Group profit") return [`${num.toFixed(2)}T ${currency}`, String(name)]
              return [`${num.toFixed(2)}%`, String(name)]
            }}
          />
          <Bar
            yAxisId="profit"
            dataKey="profit"
            name="Group profit"
            fill="#94a3b8"
            fillOpacity={0.55}
            radius={[3, 3, 0, 0]}
          />
          <Line
            yAxisId="roe"
            type="monotone"
            dataKey="roe"
            name="ROE (%)"
            stroke="#0f766e"
            strokeWidth={2.25}
            dot={{ r: 2.5, fill: "#0f766e" }}
            activeDot={{ r: 5 }}
            connectNulls
          />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  )
}
