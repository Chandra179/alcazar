import { execFileSync } from "node:child_process"
import fs from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig, type Plugin } from "vite"

const rootDirectory = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(rootDirectory, "..")
const dataRoot = path.join(repoRoot, "data")

function discoverCompanies(): string[] {
  if (!fs.existsSync(dataRoot)) return []
  return fs
    .readdirSync(dataRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .filter((name) => {
      const analysisDir = path.join(dataRoot, name, "analysis")
      return (
        fs.existsSync(path.join(analysisDir, "analysis.json")) ||
        fs.existsSync(path.join(analysisDir, "analysis.db"))
      )
    })
    .sort()
}

const reportCache = new Map<string, string>()

function loadCompanyReportJson(company: string): string | null {
  if (reportCache.has(company)) {
    return reportCache.get(company) ?? null
  }
  const analysisDir = path.join(dataRoot, company, "analysis")
  const jsonPath = path.join(analysisDir, "analysis.json")
  if (fs.existsSync(jsonPath)) {
    const raw = fs.readFileSync(jsonPath, "utf8")
    reportCache.set(company, raw)
    return raw
  }

  const dbPath = path.join(analysisDir, "analysis.db")
  if (!fs.existsSync(dbPath)) return null

  const args = ["run", "./server/cmd/bbca", "analysis", "--db", dbPath, "--format", "json"]
  const evidencePath = path.join(analysisDir, "evidence.yaml")
  if (fs.existsSync(evidencePath)) args.push("--evidence", evidencePath)

  const bankEvidencePath = path.join(analysisDir, "bank-analysis.yaml")
  if (fs.existsSync(bankEvidencePath)) {
    args.push("--bank-catalog", "bank-analysis.yaml", "--bank-evidence", bankEvidencePath)
  }
  const scenariosPath = path.join(analysisDir, "valuation-scenarios.yaml")
  if (fs.existsSync(scenariosPath)) {
    args.push("--bank-scenarios", scenariosPath)
  }

  const out = execFileSync("go", args, {
    cwd: repoRoot,
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
  })
  reportCache.set(company, out)
  return out
}

function alcazarDataPlugin(): Plugin {
  return {
    name: "alcazar-data-plugin",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const urlPath = (req.url ?? "").split("?")[0]
        if (urlPath === "/__alcazar/reports" || urlPath === "/__alcazar/reports.json") {
          const companies = discoverCompanies()
          res.setHeader("Content-Type", "application/json")
          res.end(JSON.stringify({ companies }))
          return
        }
        const match = urlPath.match(/^\/__alcazar\/reports\/([a-zA-Z0-9_-]+?)(?:\.json)?$/)
        if (match) {
          try {
            const payload = loadCompanyReportJson(match[1])
            if (!payload) {
              res.statusCode = 404
              res.end(JSON.stringify({ error: "Report not found" }))
              return
            }
            res.setHeader("Content-Type", "application/json")
            res.end(payload)
          } catch (err) {
            res.statusCode = 500
            res.end(JSON.stringify({ error: String(err) }))
          }
          return
        }
        next()
      })
    },
    closeBundle() {
      const companies = discoverCompanies()
      if (companies.length === 0) return

      const alcazarDir = path.join(rootDirectory, "dist", "__alcazar")
      const reportsDir = path.join(alcazarDir, "reports")
      const built: string[] = []

      for (const company of companies) {
        try {
          const payload = loadCompanyReportJson(company)
          if (!payload) continue
          fs.mkdirSync(reportsDir, { recursive: true })
          fs.writeFileSync(path.join(reportsDir, `${company}.json`), payload, "utf8")
          built.push(company)
        } catch (err) {
          console.warn(
            `[alcazar] skipping static report for ${company}:`,
            err instanceof Error ? err.message : String(err),
          )
        }
      }

      if (built.length > 0) {
        fs.mkdirSync(alcazarDir, { recursive: true })
        fs.writeFileSync(
          path.join(alcazarDir, "reports.json"),
          JSON.stringify({ companies: built }),
          "utf8",
        )
      }
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), alcazarDataPlugin()],
  resolve: {
    alias: {
      "@": path.resolve(rootDirectory, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (requestPath) => requestPath.replace(/^\/api/, ""),
      },
      "/swagger": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
})
