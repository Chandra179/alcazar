import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom"

import { AppShell } from "@/components/app-shell"
import { ReportProvider } from "@/lib/report-context"
import { OverviewPage } from "@/pages/overview"

export default function App() {
  return (
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
  )
}
