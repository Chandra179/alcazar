import { ChevronDown, FileText } from "lucide-react"
import { useState } from "react"

import { FinancialTable } from "@/components/financial-table"
import { SectionTakeaways } from "@/components/section-takeaways"
import { Button } from "@/components/ui/button"
import type { ReportSection } from "@/types/analysis"

interface HistorySectionProps {
  annualSections: ReportSection[]
}

export function HistorySection({ annualSections }: HistorySectionProps) {
  const [selectedYear, setSelectedYear] = useState("")
  const [showNotes, setShowNotes] = useState(false)

  if (annualSections.length === 0) return null

  const effectiveYear = selectedYear || (annualSections[0]?.period_label ?? "")
  const activeSection =
    annualSections.find((s) => s.period_label === effectiveYear) ?? annualSections[0]

  return (
    <section id="sec-history" className="scroll-mt-14 space-y-5 border-b pb-10 min-w-0">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-baseline gap-2.5">
          <h2 className="text-lg font-semibold tracking-tight">History</h2>
          <span className="font-mono text-xs text-muted-foreground">
            {annualSections.length} years
          </span>
        </div>

        <div className="flex items-center gap-2">
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
          <select
            value={effectiveYear}
            onChange={(e) => setSelectedYear(e.target.value)}
            aria-label="Select fiscal year"
            className="h-8 border-b bg-transparent px-1 font-mono text-sm focus:outline-none"
          >
            {annualSections.map((section) => {
              const yr = section.period_label ?? ""
              return (
                <option key={section.id ?? yr} value={yr} className="bg-background text-foreground">
                  FY {yr}
                </option>
              )
            })}
          </select>
        </div>
      </div>

      {activeSection ? (
        <div className="space-y-4">
          {activeSection.title ? (
            <p className="text-sm text-muted-foreground">{activeSection.title}</p>
          ) : null}

          {showNotes ? <SectionTakeaways section={activeSection} /> : null}

          {(activeSection.tables ?? []).map((table, idx) => (
            <FinancialTable key={table.title ?? idx} table={table} compact />
          ))}
        </div>
      ) : null}
    </section>
  )
}
