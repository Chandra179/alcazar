import { ExternalLink } from "lucide-react"

import { getSectionBody, getSectionSources } from "@/lib/report-data"
import type { ReportSection } from "@/types/analysis"

interface SectionTakeawaysProps {
  section: ReportSection
}

export function SectionTakeaways({ section }: SectionTakeawaysProps) {
  const body = getSectionBody(section)
  const sources = getSectionSources(section)

  return (
    <div className="space-y-3">
      {body.length > 0 ? (
        <dl className="divide-y border-y">
          {body.map((item, idx) => (
            <div key={idx} className="py-2.5 text-sm leading-relaxed">
              {item.label ? (
                <dt className="mb-0.5 text-xs font-semibold tracking-wide text-teal-700 uppercase dark:text-teal-400">
                  {item.label}
                </dt>
              ) : null}
              <dd className="text-muted-foreground">{item.text}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      {sources.length > 0 ? (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground">
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
