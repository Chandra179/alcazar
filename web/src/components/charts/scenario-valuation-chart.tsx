import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

import type { BankScenarioResult } from "@/types/analysis"

interface ScenarioValuationChartProps {
  results: BankScenarioResult[]
  currencySymbol?: string
}

export function ScenarioValuationChart({
  results,
  currencySymbol = "Rp",
}: ScenarioValuationChartProps) {
  const chartData = results.map((r) => ({
    name: r.assumptions?.name ?? r.case?.name ?? "Case",
    dividendValue: Math.round(r.dividend_value_per_share ?? r.dividend_value ?? 0),
    residualValue: Math.round(r.residual_income_value_per_share ?? r.residual_value ?? 0),
    higherRateValue: Math.round(
      r.dividend_value_at_cost_plus_1pp ?? r.dividend_value_higher_rate ?? 0,
    ),
  }))

  return (
    <div className="h-[280px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={chartData} margin={{ top: 10, right: 16, left: 4, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" vertical={false} className="stroke-border" />
          <XAxis dataKey="name" tick={{ fontSize: 12 }} tickLine={false} axisLine={false} />
          <YAxis
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v: number) => `${currencySymbol}${v.toLocaleString()}`}
            width={72}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--popover)",
              borderColor: "var(--border)",
              borderRadius: "8px",
              fontSize: "12px",
            }}
            formatter={(val, name) => [`${currencySymbol}${Number(val).toLocaleString()}`, String(name)]}
          />
          <Legend wrapperStyle={{ fontSize: "12px" }} />
          <Bar dataKey="dividendValue" name="Dividend value" fill="#0f766e" radius={[4, 4, 0, 0]} />
          <Bar dataKey="residualValue" name="Residual income" fill="#334155" radius={[4, 4, 0, 0]} />
          <Bar
            dataKey="higherRateValue"
            name="Value at +1 pp cost"
            fill="#94a3b8"
            radius={[4, 4, 0, 0]}
          />
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
