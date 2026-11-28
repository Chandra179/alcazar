import type { GlossaryEntry } from "@/types/analysis"

interface PlainTermDef extends GlossaryEntry {
  exact?: string[]
  includes?: string[]
}

const PLAIN_ENGLISH_TERMS: PlainTermDef[] = [
  {
    term: "Scope",
    group: "Reporting",
    meaning: "Does this number count just the main bank, or the whole company group?",
    exact: ["scope", "reported scope"],
  },
  {
    term: "Group",
    group: "Reporting",
    meaning: "The whole company group (the main bank plus all smaller businesses it owns).",
    exact: ["group"],
  },
  {
    term: "Bank only",
    group: "Reporting",
    meaning: "Just the main bank itself, not the other businesses it owns.",
    exact: ["bank only"],
  },
  {
    term: "Stage 2 loans",
    group: "Risk & Stress",
    meaning: "Loans where borrowers haven't defaulted yet, but are showing early signs of money trouble.",
    includes: ["stage 2"],
  },
  {
    term: "Stage 3 loans",
    group: "Risk & Stress",
    meaning: "Loans where borrowers have fallen behind or are unlikely to pay back in full.",
    includes: ["stage 3"],
  },
  {
    term: "CET1 (Common Equity Tier 1)",
    group: "Risk & Stress",
    meaning: "The bank's emergency safety cushion — money set aside to absorb sudden losses.",
    includes: ["cet1", "common equity tier 1"],
  },
  {
    term: "Risk-weighted assets (RWA)",
    group: "Risk & Stress",
    meaning: "Loans scored by how risky they are — riskier loans require a bigger safety cushion.",
    includes: ["risk-weighted assets", "rwa"],
  },
  {
    term: "EVE (Economic Value of Equity)",
    group: "Risk & Stress",
    meaning: "How much the bank's overall net worth would drop if interest rates suddenly changed.",
    includes: ["eve"],
  },
  {
    term: "NII (Net Interest Income)",
    group: "Risk & Stress",
    meaning: "Money earned from lending minus interest paid out on customer deposits.",
    includes: ["nii"],
  },
  {
    term: "Residual income",
    group: "Valuation",
    meaning: "Extra profit left over after paying a fair return to shareholders.",
    includes: ["residual income"],
  },
  {
    term: "Dividend value",
    group: "Valuation",
    meaning: "What one share is estimated to be worth based on future cash payouts.",
    includes: ["dividend value"],
  },
  {
    term: "Normalized earnings",
    group: "Valuation",
    meaning: "The bank's usual yearly profit in a normal year, without one-time surprises.",
    includes: ["normalized earnings"],
  },
  {
    term: "CASA (Low-cost deposits)",
    group: "Funding",
    meaning: "Everyday checking and savings accounts — cheap funding because they pay low interest.",
    includes: ["casa"],
  },
]

export function getPlainTermTooltip(text: string): string | undefined {
  const normalized = text.trim().toLowerCase()
  if (!normalized) return undefined

  for (const def of PLAIN_ENGLISH_TERMS) {
    if (def.exact?.includes(normalized)) return def.meaning
  }
  for (const def of PLAIN_ENGLISH_TERMS) {
    if (def.includes?.some((kw) => normalized.includes(kw))) return def.meaning
  }
  return undefined
}

export function mergePlainEnglishGlossary(existing: GlossaryEntry[]): GlossaryEntry[] {
  const seen = new Set(existing.map((entry) => entry.term.trim().toLowerCase()))
  const extras: GlossaryEntry[] = []

  for (const def of PLAIN_ENGLISH_TERMS) {
    const key = def.term.trim().toLowerCase()
    if (!seen.has(key)) {
      seen.add(key)
      extras.push({ term: def.term, meaning: def.meaning, group: def.group })
    }
  }

  return [...existing, ...extras]
}
