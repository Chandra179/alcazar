package financials

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Chandra179/lux/server/router"
	"github.com/Chandra179/lux/server/store"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestImportReviewApprovalAndReadAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestWorkbook(t, filepath.Join(dataDir, "bbca-2025.xlsx"))
	manifest := "period,document_type,local_file,format,direct_source_url\n" +
		"2025,Audited financial statements,bbca-2025.xlsx,XLSX,https://example.test/bbca-2025.xlsx\n"
	if err := os.WriteFile(filepath.Join(dataDir, "manifest.csv"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "fundamental-analysis.yaml")
	catalog := `entries:
  - id: net-profit-margin
    type: formula
    description: Net earnings as a share of revenue.
    formula: (net_earnings / revenue) * 100
  - id: income-statement
    type: concept
    description: Revenues, expenses, and profits over a period.
`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.NewSQLite(context.Background(), filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	options := ImportOptions{DataDir: dataDir, CatalogPath: catalogPath}
	first, err := Import(context.Background(), db, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.Documents != 1 || first.Facts != 7 || first.Skipped != 0 {
		t.Fatalf("first import summary = %#v", first)
	}
	var decimal string
	if err := db.QueryRow(`SELECT normalized_value FROM financial_facts WHERE canonical_key = 'total_assets' AND period = '2025'`).Scan(&decimal); err != nil {
		t.Fatal(err)
	}
	if decimal != "1.25" {
		t.Fatalf("XLSX decimal normalized as %q, want 1.25", decimal)
	}
	pending, err := ListApprovedFacts(context.Background(), db, "2025")
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || len(pending) != 0 {
		t.Fatalf("unapproved facts leaked through approved query: got %d", len(pending))
	}

	second, err := Import(context.Background(), db, options)
	if err != nil {
		t.Fatal(err)
	}
	if second.Documents != 1 || second.Facts != 0 || second.Skipped != 1 {
		t.Fatalf("repeat import summary = %#v", second)
	}

	reviewPath := filepath.Join(root, "review.csv")
	if err := ExportReviewCSV(context.Background(), db, reviewPath); err != nil {
		t.Fatal(err)
	}
	approveCandidates(t, reviewPath, map[string]bool{"net_earnings": true, "revenue": true})
	if _, err := ApplyReviewCSV(context.Background(), db, reviewPath); err != nil {
		t.Fatal(err)
	}

	approved, err := ListApprovedFacts(context.Background(), db, "2025")
	if err != nil {
		t.Fatal(err)
	}
	if len(approved) != 2 {
		t.Fatalf("approved facts = %d, want 2", len(approved))
	}
	for _, fact := range approved {
		if fact.PageNumber != 0 || fact.DocumentPath != "bbca-2025.xlsx" || fact.ExtractionMethod != "xlsx" {
			t.Fatalf("source traceability missing from fact: %#v", fact)
		}
	}

	fin := NewDependencies(&DependenciesConfig{DB: db})
	engine := router.NewDependencies(&router.DependenciesConfig{
		Logger:            zap.NewNop(),
		FinancialCoverage: fin.HandleCoverage,
		FinancialPeriods:  fin.HandlePeriods,
		FinancialFacts:    fin.HandleFacts,
		FinancialMetrics:  fin.HandleMetrics,
		FinancialPrompts:  fin.HandlePrompts,
	}).New()
	for _, route := range []string{
		"/api/v1/companies/BBCA/coverage",
		"/api/v1/companies/BBCA/periods",
		"/api/v1/companies/BBCA/facts?period=2025",
		"/api/v1/companies/BBCA/metrics?period=2025",
		"/api/v1/companies/BBCA/prompts?period=2025",
	} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest("GET", route, nil))
		if response.Code != 200 {
			t.Fatalf("GET %s status = %d, body: %s", route, response.Code, response.Body.String())
		}
	}
	metricsResponse := httptest.NewRecorder()
	engine.ServeHTTP(metricsResponse, httptest.NewRequest("GET", "/api/v1/companies/BBCA/metrics?period=2025", nil))
	var metricPayload struct {
		Metrics []Metric `json:"metrics"`
	}
	if err := json.Unmarshal(metricsResponse.Body.Bytes(), &metricPayload); err != nil {
		t.Fatal(err)
	}
	if len(metricPayload.Metrics) != 1 || metricPayload.Metrics[0].Status != "calculated" || metricPayload.Metrics[0].Value == nil || *metricPayload.Metrics[0].Value != 30 {
		t.Fatalf("metrics response = %#v", metricPayload.Metrics)
	}

	missingPeriod := httptest.NewRecorder()
	engine.ServeHTTP(missingPeriod, httptest.NewRequest("GET", "/api/v1/companies/BBCA/facts", nil))
	if missingPeriod.Code != 400 {
		t.Fatalf("facts without period status = %d, want 400", missingPeriod.Code)
	}
}

func TestForceImportRebuildsCandidatesFromCachedPDFPages(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "bbca-2025.pdf"), []byte("unchanged source hash"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "period,document_type,local_file,format,direct_source_url\n" +
		"2025,Audited financial statements,bbca-2025.pdf,PDF,https://example.test/bbca-2025.pdf\n"
	if err := os.WriteFile(filepath.Join(dataDir, "manifest.csv"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "fundamental-analysis.yaml")
	if err := os.WriteFile(catalogPath, []byte("entries:\n  - id: net-profit-margin\n    type: formula\n    description: Margin.\n    formula: earnings / revenue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.NewSQLite(context.Background(), filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := store.EnsureBBCAFinancialSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	documents, err := inventoryDocuments(dataDir)
	if err != nil || len(documents) != 1 {
		t.Fatalf("inventory documents = %d, err = %v", len(documents), err)
	}
	document := documents[0]
	_, err = db.Exec(`INSERT INTO financial_documents
		(id,sha256,local_path,source_url,period,document_type,format,audited,extraction_status)
		VALUES (?,?,?,?,?,?,?,?, 'processed')`, document.ID, document.SHA256, document.LocalPath,
		document.SourceURL, document.Period, document.DocumentType, document.Format, boolInt(document.Audited))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO financial_pages (document_id,page_number,extraction_method,text)
		VALUES (?,1,'pdftotext','2025          2024
Net profit 100 80')`, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(context.Background(), db, ImportOptions{DataDir: dataDir, CatalogPath: catalogPath, Force: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT period FROM financial_facts ORDER BY page_number, period`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var periods []string
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err != nil {
			t.Fatal(err)
		}
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(periods) != 2 || periods[0] != "2024" || periods[1] != "2025" {
		t.Fatalf("reparsed periods = %#v, want [2024 2025]", periods)
	}
}

func approveCandidates(t *testing.T, filename string, approve map[string]bool) {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(file).ReadAll()
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	columns := make(map[string]int)
	for index, name := range rows[0] {
		columns[name] = index
	}
	for index := 1; index < len(rows); index++ {
		if approve[rows[index][columns["canonical_key"]]] {
			rows[index][columns["status"]] = "approved"
		}
	}
	file, err = os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	if err := writer.WriteAll(rows); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestWorkbook(t *testing.T, filename string) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entries := map[string]string{
		"xl/workbook.xml":            `<?xml version="1.0"?><workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Statement" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?><worksheet><sheetData>
<row r="1"><c r="A1" t="inlineStr"><is><t>Net profit</t></is></c><c r="B1"><v>120</v></c></row>
<row r="2"><c r="A2" t="inlineStr"><is><t>Revenue</t></is></c><c r="B2"><v>400</v></c></row>
<row r="3"><c r="A3" t="inlineStr"><is><t>Income before tax</t></is></c><c r="B3"><v>100</v></c></row>
<row r="4"><c r="A4" t="inlineStr"><is><t>Income tax expense</t></is></c><c r="B4"><v>20</v></c></row>
<row r="5"><c r="A5" t="inlineStr"><is><t>Total liabilities</t></is></c><c r="B5"><v>900</v></c></row>
<row r="6"><c r="A6" t="inlineStr"><is><t>Total equity</t></is></c><c r="B6"><v>100</v></c></row>
<row r="7"><c r="A7" t="inlineStr"><is><t>Total assets</t></is></c><c r="B7"><v>1.250</v></c></row>
</sheetData></worksheet>`,
	}
	for name, contents := range entries {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := strings.NewReader(contents).WriteTo(writer); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
