import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type ReactNode,
} from "react"

import type { AnalysisReport } from "@/types/analysis"

interface ReportContextValue {
  report: AnalysisReport | null
  companies: string[]
  activeCompany: string
  loading: boolean
  uploadError: string
  setReport: (next: AnalysisReport) => void
  selectCompany: (company: string) => Promise<void>
  clearReport: () => void
  openFilePicker: () => void
}

const ReportContext = createContext<ReportContextValue | null>(null)

export function ReportProvider({ children }: { children: ReactNode }) {
  const [report, setReport] = useState<AnalysisReport | null>(null)
  const [companies, setCompanies] = useState<string[]>([])
  const [activeCompany, setActiveCompany] = useState("")
  const [loading, setLoading] = useState(true)
  const [uploadError, setUploadError] = useState("")
  const fileInputRef = useRef<HTMLInputElement>(null)

  const selectCompany = useCallback(async (company: string) => {
    setLoading(true)
    setUploadError("")
    try {
      const res = await fetch(`/__alcazar/reports/${encodeURIComponent(company)}`)
      if (!res.ok) throw new Error(`Failed to load ${company}`)
      const parsed = (await res.json()) as AnalysisReport
      if (parsed && Array.isArray(parsed.yearly_sections)) {
        setReport(parsed)
        setActiveCompany(company)
      }
    } catch {
      setUploadError(`Could not load ${company.toUpperCase()} from data/.`)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    async function init() {
      try {
        const res = await fetch("/__alcazar/reports")
        if (!res.ok) {
          if (!cancelled) setLoading(false)
          return
        }
        const data = (await res.json()) as { companies?: string[] }
        const list = data.companies ?? []
        if (cancelled) return
        setCompanies(list)
        if (list.length > 0) {
          await selectCompany(list[0])
        } else {
          setLoading(false)
        }
      } catch {
        if (!cancelled) setLoading(false)
      }
    }
    void init()
    return () => {
      cancelled = true
    }
  }, [selectCompany])

  function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      try {
        const parsed = JSON.parse(String(reader.result)) as AnalysisReport
        if (parsed && Array.isArray(parsed.yearly_sections)) {
          setReport(parsed)
          setActiveCompany("")
          setUploadError("")
        } else {
          setUploadError("Invalid report JSON.")
        }
      } catch {
        setUploadError("Could not parse JSON file.")
      }
    }
    reader.readAsText(file)
    event.target.value = ""
  }

  return (
    <ReportContext.Provider
      value={{
        report,
        companies,
        activeCompany,
        loading,
        uploadError,
        setReport,
        selectCompany,
        clearReport: () => {
          setReport(null)
          setActiveCompany("")
          setUploadError("")
        },
        openFilePicker: () => fileInputRef.current?.click(),
      }}
    >
      <input
        ref={fileInputRef}
        type="file"
        accept="application/json,.json"
        onChange={handleFileChange}
        className="hidden"
        aria-label="Load analysis report JSON"
      />
      {children}
    </ReportContext.Provider>
  )
}

export function useReport(): ReportContextValue {
  const ctx = useContext(ReportContext)
  if (!ctx) {
    throw new Error("useReport must be used within ReportProvider")
  }
  return ctx
}
