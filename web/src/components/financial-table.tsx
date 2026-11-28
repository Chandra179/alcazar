import { ExternalLink } from "lucide-react"

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { isGroupRow } from "@/lib/report-data"
import { cn } from "@/lib/utils"
import type { ReportCell, ReportTable } from "@/types/analysis"

interface FinancialTableProps {
  table: ReportTable
  compact?: boolean
}

function deltaToneClass(text: string): string {
  const trimmed = text.trim()
  if (trimmed.startsWith("+")) return "text-teal-700 dark:text-teal-400 font-medium"
  if (trimmed.startsWith("-") && trimmed !== "—") {
    return "text-rose-700 dark:text-rose-400 font-medium"
  }
  return ""
}

function renderCellContent(cell: ReportCell) {
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
      {cell.text ? <span>{cell.text}</span> : null}
      {cell.links?.map((link) => (
        <a
          key={`${link.text}-${link.url}`}
          href={link.url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 text-xs text-teal-700 underline-offset-2 hover:underline dark:text-teal-400"
        >
          <span>{link.text}</span>
          <ExternalLink className="size-3 shrink-0 opacity-70" />
        </a>
      ))}
    </div>
  )
}

export function FinancialTable({ table, compact = false }: FinancialTableProps) {
  const numericCols = new Set(table.numeric_columns ?? [])
  const currentCol = table.current_column ?? -1
  const changeCol = table.headers.findIndex((h) => h.toLowerCase().includes("change"))

  return (
    <div className="border-y bg-background">
      {table.title ? (
        <div className="border-b py-2.5">
          <h3 className="text-sm font-semibold tracking-tight">{table.title}</h3>
        </div>
      ) : null}
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {table.headers.map((header, idx) => (
              <TableHead
                key={`${header}-${idx}`}
                className={cn(
                  idx === 0 && "sticky left-0 z-10 bg-background pr-4",
                  numericCols.has(idx) && "text-right",
                  idx === currentCol && "bg-muted/50 text-foreground font-semibold",
                  compact && "h-8 px-2.5",
                )}
              >
                {header}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {table.rows.map((row, rowIndex) => {
            if (isGroupRow(table, rowIndex, row)) {
              return (
                <TableRow
                  key={`group-${rowIndex}`}
                  className="bg-muted/40 hover:bg-muted/40"
                >
                  <TableCell
                    colSpan={table.headers.length}
                    className={cn(
                      "sticky left-0 py-1.5 text-xs font-semibold tracking-wider text-muted-foreground uppercase",
                      compact && "px-2.5",
                    )}
                  >
                    {row[0]?.text}
                  </TableCell>
                </TableRow>
              )
            }

            return (
              <TableRow key={`row-${rowIndex}`}>
                {row.map((cell, colIndex) => {
                  const isNumeric = numericCols.has(colIndex)
                  const isCurrent = colIndex === currentCol
                  const isDelta = colIndex === changeCol

                  return (
                    <TableCell
                      key={`cell-${rowIndex}-${colIndex}`}
                      className={cn(
                        colIndex === 0 && "sticky left-0 z-10 bg-background pr-4 whitespace-nowrap",
                        compact && "px-2.5 py-2 text-xs",
                        isNumeric && "text-right font-mono tabular-nums whitespace-nowrap",
                        isCurrent && "bg-muted/30 font-semibold",
                        isDelta && deltaToneClass(cell.text),
                      )}
                    >
                      {isNumeric ? (
                        <span className="inline-block">{cell.text || "—"}</span>
                      ) : (
                        renderCellContent(cell)
                      )}
                    </TableCell>
                  )
                })}
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
