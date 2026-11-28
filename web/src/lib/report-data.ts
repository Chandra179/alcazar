import type {
  AnalysisReport,
  AnnualPoint,
  GlossaryEntry,
  ReportCell,
  ReportSection,
  ReportTable,
} from "@/types/analysis"

export function parseMetricNumber(raw: string): number | null {
  const cleaned = raw.trim()
  if (!cleaned || cleaned === "—") return null
  const normalized = cleaned
    .replace(/,/g, "")
    .replace(/^Rp\s*/i, "")
    .replace(/%$/, "")
    .replace(/×$/, "")
    .replace(/T$/i, "")
    .replace(/\s*Rp$/i, "")
    .replace(/\s*pp$/i, "")
  const num = Number.parseFloat(normalized)
  return Number.isFinite(num) ? num : null
}

export function getReportTicker(report: AnalysisReport): string {
  const title = report.yearly_template?.title ?? ""
  const token = title.trim().split(/\s+/)[0] ?? ""
  return token ? token.toUpperCase() : "REPORT"
}

export function getReportTitle(report: AnalysisReport): string {
  return report.yearly_template?.title || "Financial Analysis"
}

export function getSectionById(report: AnalysisReport, id: string): ReportSection | undefined {
  return (report.yearly_sections ?? []).find((section) => section.id === id)
}

export function getSectionsByKind(report: AnalysisReport, kind: string): ReportSection[] {
  return (report.yearly_sections ?? []).filter((section) => section.kind === kind)
}

export function getSectionBody(section: ReportSection): ReportCell[] {
  const paragraphs = section.paragraphs ?? []
  const sourceRows = new Set(section.layout?.source_rows ?? [])
  return paragraphs.filter((_, index) => !sourceRows.has(index))
}

export function getSectionSources(section: ReportSection): ReportCell[] {
  const paragraphs = section.paragraphs ?? []
  const sourceRows = section.layout?.source_rows ?? []
  return sourceRows.map((index) => paragraphs[index]).filter(Boolean)
}

export function isGroupRow(table: ReportTable, rowIndex: number, row: ReportCell[]): boolean {
  if (table.layout?.group_rows) {
    return table.layout.group_rows.includes(rowIndex)
  }
  if (row.length === 0 || !row[0].text) return false
  return row.slice(1).every((cell) => !cell.text && (!cell.links || cell.links.length === 0))
}

export function extractAnnualSeries(report: AnalysisReport): AnnualPoint[] {
  const annuals = getSectionsByKind(report, "annual").slice().reverse()
  const points: AnnualPoint[] = []

  for (const section of annuals) {
    const year = Number.parseInt(section.period_label ?? "", 10)
    if (!Number.isFinite(year)) continue

    const point: AnnualPoint = {
      year,
      label: String(year),
      profit: null,
      loans: null,
      deposits: null,
      casa: null,
      nim: null,
      roe: null,
      roa: null,
      npl: null,
      car: null,
      divYield: null,
      pe: null,
      pbv: null,
      totalReturn: null,
      profitIndex: null,
      returnIndex: null,
    }

    for (const table of section.tables ?? []) {
      const col =
        table.current_column && table.current_column > 0
          ? table.current_column
          : Math.max(0, table.headers.length - 1)

      for (let i = 0; i < table.rows.length; i++) {
        const row = table.rows[i]
        if (row.length <= col || isGroupRow(table, i, row)) continue
        const name = row[0].text.trim()
        const val = parseMetricNumber(row[col].text)
        if (val === null) continue

        switch (name) {
          case "Group profit":
            point.profit = val
            break
          case "Loans (group)":
            point.loans = val
            break
          case "Customer deposits (group)":
            point.deposits = val
            break
          case "Low-cost deposit mix (CASA) (group)":
            point.casa = val
            break
          case "Net interest margin (bank only)":
            point.nim = val
            break
          case "Return on equity":
            point.roe = val
            break
          case "Return on assets":
            point.roa = val
            break
          case "Gross non-performing loans (bank only)":
            point.npl = val
            break
          case "Capital adequacy ratio (group)":
            point.car = val
            break
          case "Dividend yield":
            point.divYield = val
            break
          case "Price / earnings":
            point.pe = val
            break
          case "Price / book value":
            point.pbv = val
            break
          case "Return including cash dividends":
            point.totalReturn = val
            break
        }
      }
    }

    points.push(point)
  }

  // Index profit & cumulative total return from the first year with positive profit & ROE
  const baseIndex = points.findIndex((p) => p.profit !== null && p.profit > 0 && p.roe !== null)
  if (baseIndex >= 0) {
    const baseProfit = points[baseIndex].profit!
    let cumulativeReturn = 100
    for (let i = baseIndex; i < points.length; i++) {
      const p = points[i]
      if (p.profit !== null && p.profit > 0) {
        p.profitIndex = Number(((p.profit / baseProfit) * 100).toFixed(2))
      }
      if (i === baseIndex) {
        p.returnIndex = 100
      } else if (p.totalReturn !== null) {
        cumulativeReturn = cumulativeReturn * (1 + p.totalReturn / 100)
        p.returnIndex = Number(cumulativeReturn.toFixed(2))
      }
    }
  }

  return points
}

export function computeMedian(values: (number | null)[]): number | null {
  const valid = values.filter((v): v is number => v !== null).sort((a, b) => a - b)
  if (valid.length === 0) return null
  const mid = Math.floor(valid.length / 2)
  return valid.length % 2 === 0 ? (valid[mid - 1] + valid[mid]) / 2 : valid[mid]
}

export function extractRowValue(section: ReportSection | undefined, measurePrefix: string): string {
  if (!section) return "—"
  for (const table of section.tables ?? []) {
    const col =
      table.current_column && table.current_column > 0
        ? table.current_column
        : Math.max(0, table.headers.length - 1)
    for (const row of table.rows) {
      if (row[0]?.text.startsWith(measurePrefix) && row[col]?.text) {
        return row[col].text
      }
    }
  }
  return "—"
}

export function extractRowDelta(section: ReportSection | undefined, measurePrefix: string): string {
  if (!section) return ""
  for (const table of section.tables ?? []) {
    const changeCol = table.headers.findIndex((h) => h.toLowerCase() === "change")
    if (changeCol < 0) continue
    for (const row of table.rows) {
      if (row[0]?.text.startsWith(measurePrefix) && row[changeCol]?.text) {
        return row[changeCol].text
      }
    }
  }
  return ""
}

export function extractGlossaryTerms(report: AnalysisReport): GlossaryEntry[] {
  const glossary = getSectionById(report, "yearly-glossary")
  const table = glossary?.tables?.[0]
  if (!table) return []

  const entries: GlossaryEntry[] = []
  let currentGroup: string | undefined

  for (let i = 0; i < table.rows.length; i++) {
    const row = table.rows[i]
    if (isGroupRow(table, i, row)) {
      currentGroup = row[0]?.text.trim() || undefined
      continue
    }
    const term = row[0]?.text.trim()
    const meaning = row[1]?.text.trim()
    if (term && meaning) {
      entries.push({ term, meaning, group: currentGroup })
    }
  }
  return entries
}
