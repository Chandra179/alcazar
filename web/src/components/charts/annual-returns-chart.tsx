import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

import type { AnnualPoint } from "@/types/analysis"

interface AnnualReturnsChartProps {
  data: AnnualPoint[]
}

export function AnnualReturnsChart({ data }: AnnualReturnsChartProps) {
  const filtered = data.filter((d) => d.totalReturn !== null)

  return (
    <div className="h-[300px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={filtered} margin={{ top: 10, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" vertical={false} className="stroke-border" />
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            minTickGap={14}
          />
          <YAxis
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v: number) => `${v}%`}
            width={44}
          />
          <ReferenceLine y={0} stroke="#64748b" strokeWidth={1} />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--popover)",
              borderColor: "var(--border)",
              borderRadius: "8px",
              fontSize: "12px",
            }}
            formatter={(val) => {
              const num = Number(val)
              const sign = num > 0 ? "+" : ""
              return [`${sign}${num.toFixed(2)}%`, "Total return"]
            }}
          />
          <Bar dataKey="totalReturn" name="Total return" radius={[3, 3, 0, 0]}>
            {filtered.map((entry) => (
              <Cell
                key={entry.label}
                fill={(entry.totalReturn ?? 0) >= 0 ? "#0f766e" : "#be123c"}
              />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
