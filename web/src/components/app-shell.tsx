import {
  ArrowUpRight,
  BookOpen,
  CalendarRange,
  Hexagon,
  LayoutDashboard,
  Menu,
  Moon,
  MoreHorizontal,
  ShieldAlert,
  Sun,
  Trash2,
  TrendingUp,
  Upload,
  X,
} from "lucide-react"
import { useEffect, useState } from "react"
import { Outlet } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { useReport } from "@/lib/report-context"
import {
  extractAnnualSeries,
  extractGlossaryTerms,
  getReportTicker,
  getSectionById,
  getSectionsByKind,
} from "@/lib/report-data"

export function AppShell() {
  const [dark, setDark] = useState(false)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const { report, companies, activeCompany, uploadError, selectCompany, clearReport, openFilePicker } =
    useReport()

  const ticker = report ? getReportTicker(report) : ""
  const annualSections = report ? getSectionsByKind(report, "annual") : []
  const annualSeries = report ? extractAnnualSeries(report) : []
  const hasRatios = annualSeries.some(
    (p) =>
      p.roe !== null ||
      p.roa !== null ||
      p.pe !== null ||
      p.pbv !== null ||
      p.divYield !== null ||
      p.nim !== null ||
      p.totalReturn !== null,
  )
  const hasRiskOrScenarios = Boolean(
    report &&
      (getSectionById(report, "yearly-bank-risk") ||
        getSectionById(report, "yearly-bank-scenarios") ||
        report.bank_scenarios?.results?.length),
  )
  const hasAudit = Boolean(
    report && (extractGlossaryTerms(report).length > 0 || (report.rules?.length ?? 0) > 0),
  )

  const navItems = report
    ? [
        { id: "sec-overview", label: "Overview", icon: LayoutDashboard },
        ...(hasRatios ? [{ id: "sec-ratios", label: "Ratios", icon: TrendingUp }] : []),
        ...(hasRiskOrScenarios
          ? [{ id: "sec-risk", label: "Risk & Stress", icon: ShieldAlert }]
          : []),
        ...(annualSections.length > 0
          ? [{ id: "sec-history", label: "History", icon: CalendarRange }]
          : []),
        ...(hasAudit ? [{ id: "sec-audit", label: "Glossary & Audit", icon: BookOpen }] : []),
      ]
    : []

  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark)
  }, [dark])

  const firstYear = annualSeries[0]?.label
  const lastYear = annualSeries[annualSeries.length - 1]?.label
  const span = firstYear && lastYear ? `FY${firstYear}–${lastYear}` : "No report"

  return (
    <div className="flex min-h-svh w-full bg-background">
      {/* Left Navigation Sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-40 flex w-56 flex-col justify-between border-r bg-background transition-transform duration-200 md:sticky md:top-0 md:h-svh md:shrink-0 md:translate-x-0 ${
          drawerOpen ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        <div>
          <div className="flex items-center justify-between border-b px-4 py-3.5">
            <a
              href="#sec-overview"
              onClick={() => setDrawerOpen(false)}
              className="flex min-w-0 items-center gap-2"
            >
              <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-teal-700 text-white dark:bg-teal-600">
                <Hexagon className="size-3.5" />
              </span>
              <span className="text-sm font-semibold tracking-tight">Alcazar</span>
            </a>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              className="md:hidden"
              onClick={() => setDrawerOpen(false)}
              aria-label="Close navigation"
            >
              <X className="size-4" />
            </Button>
          </div>

          <nav className="space-y-0.5 p-2.5" aria-label="Report sections">
            {companies.length > 1 ? (
              <div className="mb-2 px-1">
                <select
                  value={activeCompany}
                  onChange={(e) => {
                    void selectCompany(e.target.value)
                    setDrawerOpen(false)
                  }}
                  aria-label="Select company from data directory"
                  className="w-full border-b bg-transparent py-1.5 font-mono text-xs font-semibold uppercase focus:outline-none"
                >
                  {companies.map((c) => (
                    <option key={c} value={c} className="bg-background text-foreground">
                      {c.toUpperCase()}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
            {navItems.map((item) => {
              const Icon = item.icon
              return (
                <a
                  key={item.id}
                  href={`#${item.id}`}
                  onClick={() => setDrawerOpen(false)}
                  className="flex items-center gap-2.5 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <Icon className="size-4 shrink-0 opacity-75" />
                  <span className="truncate">{item.label}</span>
                </a>
              )
            })}
          </nav>
        </div>

        <div className="relative border-t px-3 py-2.5">
          {uploadError ? (
            <p className="mb-2 text-xs text-rose-700 dark:text-rose-400">{uploadError}</p>
          ) : null}
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span className="truncate font-mono">{span}</span>
            <div className="flex items-center gap-0.5">
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                onClick={openFilePicker}
                title="Load JSON report"
                aria-label="Load JSON report"
              >
                <Upload className="size-3.5" />
              </Button>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                onClick={() => setDark((prev) => !prev)}
                title={dark ? "Light mode" : "Dark mode"}
                aria-label={dark ? "Switch to light mode" : "Switch to dark mode"}
              >
                {dark ? <Sun className="size-3.5" /> : <Moon className="size-3.5" />}
              </Button>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                onClick={() => setMenuOpen((prev) => !prev)}
                title="More actions"
                aria-label="More actions"
              >
                <MoreHorizontal className="size-3.5" />
              </Button>
            </div>
          </div>

          {menuOpen ? (
            <div className="absolute right-3 bottom-12 z-50 w-44 rounded-md border bg-popover py-1 text-xs shadow-md">
              <button
                type="button"
                onClick={() => {
                  openFilePicker()
                  setMenuOpen(false)
                }}
                className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-accent"
              >
                <Upload className="size-3.5" />
                Load JSON
              </button>
              {report ? (
                <button
                  type="button"
                  onClick={() => {
                    clearReport()
                    setMenuOpen(false)
                  }}
                  className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-accent"
                >
                  <Trash2 className="size-3.5" />
                  Clear report
                </button>
              ) : null}
              <a
                href="/swagger/index.html"
                target="_blank"
                rel="noreferrer"
                onClick={() => setMenuOpen(false)}
                className="flex w-full items-center justify-between px-3 py-1.5 hover:bg-accent"
              >
                <span>API docs</span>
                <ArrowUpRight className="size-3.5 opacity-70" />
              </a>
            </div>
          ) : null}
        </div>
      </aside>

      {drawerOpen ? (
        <div
          className="fixed inset-0 z-30 bg-black/30 md:hidden"
          onClick={() => setDrawerOpen(false)}
          aria-hidden="true"
        />
      ) : null}

      {/* Main Content Area */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/* Minimal Mobile Header */}
        <header className="sticky top-0 z-20 flex items-center justify-between border-b bg-background/95 px-4 py-2.5 backdrop-blur md:hidden">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              onClick={() => setDrawerOpen(true)}
              aria-label="Open menu"
            >
              <Menu className="size-4" />
            </Button>
            <span className="text-sm font-semibold tracking-tight">Alcazar</span>
          </div>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              onClick={openFilePicker}
              aria-label="Load JSON report"
            >
              <Upload className="size-4" />
            </Button>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              onClick={() => setDark((prev) => !prev)}
              aria-label={dark ? "Switch to light mode" : "Switch to dark mode"}
            >
              {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </Button>
          </div>
        </header>

        <main className="mx-auto w-full max-w-5xl min-w-0 px-4 py-6 sm:px-8 sm:py-10">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
