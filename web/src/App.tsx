import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom"

import { AppShell } from "@/components/app-shell"
import { TooltipProvider } from "@/components/ui/tooltip"
import { ReportProvider } from "@/lib/report-context"
import { OverviewPage } from "@/pages/overview"

export default function App() {
  return (
    <TooltipProvider delayDuration={200}>
      <ReportProvider>
        <BrowserRouter>
          <Routes>
            <Route element={<AppShell />}>
              <Route index element={<OverviewPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </ReportProvider>
    </TooltipProvider>
  )
}
