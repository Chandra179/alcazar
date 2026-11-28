import { ExternalLink } from "lucide-react"

import { getSectionBody, getSectionSources } from "@/lib/report-data"
import type { ReportSection } from "@/types/analysis"

interface SectionTakeawaysProps {
  section: ReportSection
  showBody?: boolean
  showSources?: boolean
  compactParagraph?: boolean
}

export function SectionTakeaways({
  section,
  showBody = true,
  showSources = true,
  compactParagraph = false,
}: SectionTakeawaysProps) {
  const body = showBody ? getSectionBody(section) : []
  const sources = showSources ? getSectionSources(section) : []

  if (body.length === 0 && sources.length === 0) return null

  return (
    <div className="space-y-2.5">
      {body.length > 0 ? (
        compactParagraph ? (
          <p className="max-w-3xl text-[13px] leading-relaxed text-muted-foreground">
            {body.map((item) => item.text).filter(Boolean).join(" ")}
          </p>
        ) : (
          <div className="space-y-2">
            {body.map((item, idx) => (
              <p
                key={idx}
                className="max-w-3xl text-[13px] leading-relaxed text-muted-foreground"
              >
                {item.text}
              </p>
            ))}
          </div>
        )
      ) : null}

      {sources.length > 0 ? (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pt-0.5 text-xs text-muted-foreground">
          <span className="font-medium uppercase tracking-wide">Sources:</span>
          {sources.flatMap((src) =>
            (src.links ?? []).map((link) => (
              <a
                key={`${link.text}-${link.url}`}
                href={link.url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-teal-700 hover:underline dark:text-teal-400"
              >
                <span>{link.text}</span>
                <ExternalLink className="size-3 opacity-70" />
              </a>
            )),
          )}
        </div>
      ) : null}
    </div>
  )
}
