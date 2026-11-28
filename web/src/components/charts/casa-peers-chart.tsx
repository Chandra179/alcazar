import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

export interface CasaPeerPoint {
  bank: string
  share: number
  scope: string
}

interface CasaPeersChartProps {
  data: CasaPeerPoint[]
  primaryTicker?: string
}

export function CasaPeersChart({ data, primaryTicker = "" }: CasaPeersChartProps) {
  return (
    <div className="h-[220px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart
          data={data}
          layout="vertical"
          margin={{ top: 6, right: 24, left: 10, bottom: 6 }}
        >
          <CartesianGrid strokeDasharray="3 3" horizontal={false} className="stroke-border" />
          <XAxis
            type="number"
            domain={[0, 100]}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v: number) => `${v}%`}
          />
          <YAxis
            type="category"
            dataKey="bank"
            tick={{ fontSize: 12, fontWeight: 500 }}
            tickLine={false}
            axisLine={false}
            width={64}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--popover)",
              borderColor: "var(--border)",
              borderRadius: "8px",
              fontSize: "12px",
            }}
            formatter={(val, _name, item) => [
              `${Number(val).toFixed(1)}% (${(item?.payload as CasaPeerPoint)?.scope ?? ""})`,
              "CASA Share",
            ]}
          />
          <Bar dataKey="share" radius={[0, 4, 4, 0]} barSize={24}>
            {data.map((entry) => (
              <Cell
                key={entry.bank}
                fill={
                  primaryTicker && entry.bank.toUpperCase().includes(primaryTicker.toUpperCase())
                    ? "#0f766e"
                    : "#94a3b8"
                }
              />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
