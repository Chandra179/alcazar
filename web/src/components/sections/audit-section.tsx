import { ChevronDown, ExternalLink, Search } from "lucide-react"
import { useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { AnalysisRule, AnalysisSource, GlossaryEntry } from "@/types/analysis"

interface AuditSectionProps {
  glossary: GlossaryEntry[]
  rules: AnalysisRule[]
  sources: Record<string, AnalysisSource>
}

function statusVariant(status: string): "positive" | "warning" | "negative" | "outline" {
  const s = status.toLowerCase()
  if (s === "pass" || s === "verified" || s === "supported") return "positive"
  if (s.includes("partial") || s.includes("gap")) return "warning"
  if (s === "fail" || s === "missing") return "negative"
  return "outline"
}

export function AuditSection({ glossary, rules, sources }: AuditSectionProps) {
  const [expanded, setExpanded] = useState(false)
  const [tab, setTab] = useState<"glossary" | "rules">(
    glossary.length > 0 ? "glossary" : "rules",
  )
  const [query, setQuery] = useState("")

  const filteredGlossary = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return glossary
    return glossary.filter(
      (g) =>
        g.term.toLowerCase().includes(q) ||
        g.meaning.toLowerCase().includes(q) ||
        (g.group ?? "").toLowerCase().includes(q),
    )
  }, [glossary, query])

  const filteredRules = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rules
    return rules.filter(
      (r) =>
        r.rule.id.toLowerCase().includes(q) ||
        (r.rule.title ?? "").toLowerCase().includes(q) ||
        r.finding.toLowerCase().includes(q),
    )
  }, [rules, query])

  if (glossary.length === 0 && rules.length === 0) return null

  return (
    <section id="sec-audit" className="scroll-mt-14 space-y-4 min-w-0">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-baseline gap-2.5">
          <h2 className="text-lg font-semibold tracking-tight">Glossary &amp; Audit</h2>
          <span className="font-mono text-xs text-muted-foreground">
            {glossary.length} terms · {rules.length} rules
          </span>
        </div>

        <Button
          type="button"
          size="xs"
          variant="ghost"
          onClick={() => setExpanded((prev) => !prev)}
        >
          <span>{expanded ? "Hide" : "Show"}</span>
          <ChevronDown
            className={`size-3.5 transition-transform ${expanded ? "rotate-180" : ""}`}
          />
        </Button>
      </div>

      {expanded ? (
        <div className="space-y-4 pt-1">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-1.5">
              {glossary.length > 0 ? (
                <Button
                  type="button"
                  size="xs"
                  variant={tab === "glossary" ? "secondary" : "ghost"}
                  onClick={() => setTab("glossary")}
                >
                  Glossary ({glossary.length})
                </Button>
              ) : null}
              {rules.length > 0 ? (
                <Button
                  type="button"
                  size="xs"
                  variant={tab === "rules" ? "secondary" : "ghost"}
                  onClick={() => setTab("rules")}
                >
                  Rules ({rules.length})
                </Button>
              ) : null}
            </div>

            <div className="relative w-full sm:w-64">
              <Search className="pointer-events-none absolute top-2 left-2.5 size-3.5 text-muted-foreground" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Filter..."
                className="h-8 pl-8 text-xs"
              />
            </div>
          </div>

          {tab === "glossary" ? (
            <div className="border-y">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="sticky left-0 z-10 bg-background pr-4">Term</TableHead>
                    <TableHead>Meaning</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredGlossary.map((item) => (
                    <TableRow key={item.term}>
                      <TableCell className="sticky left-0 z-10 bg-background pr-4 font-medium whitespace-nowrap">
                        {item.term}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{item.meaning}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : (
            <div className="max-h-[480px] overflow-y-auto border-y">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="w-10">#</TableHead>
                    <TableHead className="sticky left-0 z-10 bg-background">Rule</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Finding</TableHead>
                    <TableHead>Sources</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredRules.map((item) => (
                    <TableRow key={item.rule.id}>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {item.number}
                      </TableCell>
                      <TableCell className="sticky left-0 z-10 bg-background align-top whitespace-nowrap">
                        <div className="text-xs font-medium">
                          {item.rule.title ?? item.rule.id}
                        </div>
                      </TableCell>
                      <TableCell className="align-top">
                        <Badge variant={statusVariant(item.status)}>{item.status}</Badge>
                      </TableCell>
                      <TableCell className="max-w-md align-top text-xs">
                        <p>{item.finding}</p>
                        {item.gap ? (
                          <p className="mt-1 text-amber-700 dark:text-amber-400">
                            Gap: {item.gap}
                          </p>
                        ) : null}
                      </TableCell>
                      <TableCell className="align-top">
                        <div className="flex flex-col gap-1">
                          {(item.sources ?? []).slice(0, 2).map((srcKey) => {
                            const src = sources[srcKey]
                            if (!src) return null
                            return (
                              <a
                                key={srcKey}
                                href={src.url}
                                target="_blank"
                                rel="noreferrer"
                                className="inline-flex items-center gap-1 text-xs text-teal-700 hover:underline dark:text-teal-400"
                              >
                                <span className="max-w-[140px] truncate">{src.title}</span>
                                <ExternalLink className="size-3 shrink-0 opacity-70" />
                              </a>
                            )
                          })}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      ) : null}
    </section>
  )
}
