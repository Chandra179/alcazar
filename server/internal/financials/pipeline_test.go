package financials

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Chandra179/lux/server/router"
	"github.com/Chandra179/lux/server/store"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
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
	if err := ExportReviewCSV(context.Background(), db, reviewPath, ReviewFilter{}); err != nil {
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
	options.Force = true
	if _, err := Import(context.Background(), db, options); err != nil {
		t.Fatal(err)
	}
	replayed, err := ListApprovedFacts(context.Background(), db, "2025")
	if err != nil || len(replayed) != len(approved) {
		t.Fatalf("replay changed approvals: %#v, %v", replayed, err)
	}
	if _, err := ApplyReviewCSV(context.Background(), db, reviewPath); err == nil {
		t.Fatal("previously applied review CSV was accepted")
	}
	for _, fact := range approved {
		if fact.PageNumber != 0 || fact.SourceLocator == "" || fact.DocumentPath != "bbca-2025.xlsx" || fact.ExtractionMethod != "xlsx" {
			t.Fatalf("source traceability missing from fact: %#v", fact)
		}
		if fact.CanonicalKey == "net_earnings" && fact.SourceLocator != "Statement!B1" {
			t.Fatalf("XLSX source locator = %q, want Statement!B1", fact.SourceLocator)
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
	factsResponse := httptest.NewRecorder()
	engine.ServeHTTP(factsResponse, httptest.NewRequest("GET", "/api/v1/companies/BBCA/facts?period=2025", nil))
	var factsPayload struct {
		Facts []Fact `json:"facts"`
	}
	if err := json.Unmarshal(factsResponse.Body.Bytes(), &factsPayload); err != nil {
		t.Fatal(err)
	}
	if len(factsPayload.Facts) != 2 || factsPayload.Facts[0].SourceLocator == "" || factsPayload.Facts[1].SourceLocator == "" {
		t.Fatalf("API facts lost workbook source locators: %#v", factsPayload.Facts)
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
		VALUES (?,1,'pdftotext','PT BANK CENTRAL ASIA Tbk AND SUBSIDIARIES
CONSOLIDATED STATEMENTS OF PROFIT OR LOSS
(Expressed in millions of Rupiah)
Notes          2025          2024
Net profit     100           80')`, document.ID)
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

func TestReviewPromptsFindComparativePeriodEvidence(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "fundamental-analysis.yaml")
	catalog := `entries:
  - id: cash-flow-statement
    type: concept
    description: Cash flow statement reports sources and uses of cash.
  - id: credit-risk
    type: heuristic
    description: Credit risk depends on borrowers' repayment capacity.
`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.NewSQLite(context.Background(), filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := Initialize(context.Background(), db, catalogPath); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO financial_documents
		(id,sha256,local_path,source_url,period,document_type,format,audited,extraction_status)
		VALUES ('doc-2002','hash-2002','annual_reports/BCA_AR_2002.pdf','https://example.test/2002.pdf',
		'2002','Annual report','PDF',1,'processed')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO financial_pages (document_id,page_number,extraction_method,text)
		VALUES ('doc-2002',2,'pdftotext','Five-year cash flow highlights for 1998 through 2002'),
		('doc-2002',3,'pdftotext','Unrelated governance information')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO financial_facts
		(id,document_id,page_number,period,raw_label,raw_value,evidence_text,canonical_key,normalized_value,
		currency,unit,reporting_basis,audited_status,extraction_method,status)
		VALUES ('fact-1998','doc-2002',2,'1998','Net cash from operations','10','Net cash from operations 10',
		'net_cash_from_operating_activities','10','IDR','millions','consolidated','audited','pdftotext','pending')`)
	if err != nil {
		t.Fatal(err)
	}
	prompts, err := ListReviewPrompts(context.Background(), db, "1998")
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 || prompts[0].Entity.ID != "cash-flow-statement" || len(prompts[0].Evidence) != 1 {
		t.Fatalf("FY1998 prompts = %#v, want only the evidence-linked cash flow prompt", prompts)
	}
	if prompts[0].Evidence[0].PageNumber != 2 || prompts[0].Evidence[0].DocumentPath != "annual_reports/BCA_AR_2002.pdf" {
		t.Fatalf("FY1998 evidence source = %#v, want FY2002 annual report page 2", prompts[0].Evidence[0])
	}
	missing, err := ListReviewPrompts(context.Background(), db, "1997")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("prompts without evidence = %#v, want empty", missing)
	}
}

func TestCalculateMetricsDerivesAverageBalanceInputs(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "fundamental-analysis.yaml")
	catalog := `entries:
  - id: return-on-assets
    type: formula
    description: Net earnings divided by average total assets.
    formula: (net_earnings / average_total_assets) * 100
  - id: return-on-equity
    type: formula
    description: Net earnings available to common shareholders divided by average common equity.
    formula: (net_income_available_to_common_shareholders / average_common_shareholders_equity) * 100
`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.NewSQLite(context.Background(), filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := Initialize(context.Background(), db, catalogPath); err != nil {
		t.Fatal(err)
	}
	for _, document := range []struct{ id, hash, period string }{
		{id: "doc-2025", hash: "hash-2025", period: "2025"},
		{id: "doc-2024", hash: "hash-2024", period: "2024"},
	} {
		_, err := db.Exec(`INSERT INTO financial_documents
			(id,sha256,local_path,period,document_type,format,audited,extraction_status)
			VALUES (?,?,?,?,?,?,1,'processed')`, document.id, document.hash,
			"report-"+document.period+".pdf", document.period, "Audited financial statements", "PDF")
		if err != nil {
			t.Fatal(err)
		}
	}
	facts := []struct{ id, doc, period, key, label, value string }{
		{"net-earnings", "doc-2025", "2025", "net_earnings", "Net earnings", "100"},
		{"common-earnings", "doc-2025", "2025", "net_income_available_to_common_shareholders", "Net income available to common shareholders", "90"},
		{"assets-end", "doc-2025", "2025", "total_assets", "Total assets", "200"},
		{"equity-end", "doc-2025", "2025", "common_shareholders_equity", "Common equity", "150"},
		{"assets-start", "doc-2024", "2024", "total_assets", "Total assets", "100"},
		{"equity-start", "doc-2024", "2024", "common_shareholders_equity", "Common equity", "100"},
	}
	for _, fact := range facts {
		_, err := db.Exec(`INSERT INTO financial_facts
			(id,document_id,page_number,period,raw_label,raw_value,evidence_text,canonical_key,normalized_value,
			currency,unit,reporting_basis,audited_status,extraction_method,status)
			VALUES (?,?,1,?,?,?,?,?,?, 'IDR','millions','consolidated','audited','review','approved')`,
			fact.id, fact.doc, fact.period, fact.label, fact.value, fact.label+" "+fact.value,
			fact.key, fact.value)
		if err != nil {
			t.Fatal(err)
		}
	}
	metrics, err := CalculateMetrics(context.Background(), db, "2025")
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]Metric, len(metrics))
	for _, metric := range metrics {
		byID[metric.EntityID] = metric
	}
	if metric := byID["return-on-assets"]; metric.Status != "calculated" || metric.Value == nil || math.Abs(*metric.Value-(100.0/150*100)) > 1e-9 {
		t.Fatalf("return-on-assets = %#v", metric)
	}
	if metric := byID["return-on-equity"]; metric.Status != "calculated" || metric.Value == nil || math.Abs(*metric.Value-72) > 1e-9 {
		t.Fatalf("return-on-equity = %#v", metric)
	}
	averageInput, ok := byID["return-on-assets"].Inputs["average_total_assets"].(map[string]any)
	if !ok {
		t.Fatalf("average asset input = %#v", byID["return-on-assets"].Inputs["average_total_assets"])
	}
	sources, ok := averageInput["source_facts"].([]map[string]any)
	if !ok || len(sources) != 2 || sources[0]["fact_id"] != "assets-end" || sources[1]["fact_id"] != "assets-start" {
		t.Fatalf("average asset sources = %#v", averageInput["source_facts"])
	}
}

func TestCapitalExpenditureBurdenUsesTrailingTenAnnualPeriods(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "fundamental-analysis.yaml")
	catalog := `entries:
  - id: capital-expenditure-to-net-earnings
    type: formula
    description: Ten-year reinvestment burden.
    formula: (sum(capital_expenditures over period) / sum(net_earnings over same period)) * 100
`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.NewSQLite(context.Background(), filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := Initialize(context.Background(), db, catalogPath); err != nil {
		t.Fatal(err)
	}
	for year := 2016; year <= 2025; year++ {
		period := strconv.Itoa(year)
		documentID := "doc-" + period
		_, err := db.Exec(`INSERT INTO financial_documents
			(id,sha256,local_path,source_url,period,document_type,format,audited,extraction_status)
			VALUES (?,?,?,?,?,'Audited financial statements','PDF',1,'processed')`,
			documentID, "hash-"+period, "report-"+period+".pdf", "https://example.test/"+period, period)
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range []struct{ key, label, value string }{
			{"capital_expenditures", "Capital expenditures", "-1"},
			{"net_earnings", "Net earnings", "10"},
		} {
			factID := input.key + "-" + period
			_, err := db.Exec(`INSERT INTO financial_facts
				(id,document_id,page_number,period,raw_label,raw_value,evidence_text,canonical_key,normalized_value,
				currency,unit,reporting_basis,audited_status,extraction_method,status)
				VALUES (?,?,8,?,?,?,?,?,?,'IDR','millions','consolidated','audited','review','approved')`,
				factID, documentID, period, input.label, input.value, input.label+" "+input.value,
				input.key, input.value)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	metrics, err := CalculateMetrics(context.Background(), db, "2025")
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("metrics = %#v, want only capital expenditure burden", metrics)
	}
	metric := metrics[0]
	if metric.Status != "calculated" || metric.Value == nil || math.Abs(*metric.Value-10) > 1e-9 {
		t.Fatalf("ten-year capital expenditure burden = %#v, want 10%%", metric)
	}
	if metric.Unit != "percent" || metric.Inputs["window_start"] != "2016" || metric.Inputs["window_end"] != "2025" {
		t.Fatalf("metric window metadata = %#v", metric.Inputs)
	}
	annualInputs, ok := metric.Inputs["annual_inputs"].([]map[string]any)
	if !ok || len(annualInputs) != 10 {
		t.Fatalf("annual inputs = %#v, want ten periods", metric.Inputs["annual_inputs"])
	}
	capexInput, ok := annualInputs[0]["capital_expenditures"].(map[string]any)
	if !ok || capexInput["value"] != float64(-1) {
		t.Fatalf("first-year capex evidence = %#v", annualInputs[0]["capital_expenditures"])
	}
	sources, ok := capexInput["source_facts"].([]map[string]any)
	if !ok || len(sources) != 1 || sources[0]["fact_id"] != "capital_expenditures-2016" || sources[0]["page_number"] != 8 {
		t.Fatalf("capex source references = %#v", capexInput["source_facts"])
	}
	if _, err := db.Exec(`UPDATE financial_facts SET reporting_basis='unknown' WHERE id='capital_expenditures-2016'`); err != nil {
		t.Fatal(err)
	}
	incompatible, err := CalculateMetrics(context.Background(), db, "2025")
	if err != nil {
		t.Fatal(err)
	}
	if len(incompatible) != 1 || incompatible[0].Status != "conflict" {
		t.Fatalf("mixed-metadata capital expenditure burden = %#v, want conflict", incompatible)
	}

	interim, err := CalculateMetrics(context.Background(), db, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if len(interim) != 1 || interim[0].Status != "requires_multi_period" || interim[0].Value != nil {
		t.Fatalf("interim capital expenditure burden = %#v, want requires_multi_period", interim)
	}

	missingWindow, err := CalculateMetrics(context.Background(), db, "2015")
	if err != nil {
		t.Fatal(err)
	}
	if len(missingWindow) != 1 || missingWindow[0].Status != "missing_inputs" {
		t.Fatalf("incomplete historical window = %#v, want missing_inputs", missingWindow)
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
			for field, value := range map[string]string{
				"currency": "IDR", "unit": "millions", "reporting_basis": "consolidated", "audited_status": "audited",
			} {
				if column, ok := columns[field]; ok {
					rows[index][column] = value
				}
			}
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
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	if err := file.SetSheetName("Sheet1", "Statement"); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"Net profit", 120},
		{"Revenue", 400},
		{"Income before tax", 100},
		{"Income tax expense", 20},
		{"Total liabilities", 900},
		{"Total equity", 100},
		{"Total assets", 1.25},
		{"Adjustment for portfolio already deducted from total assets", 3},
	}
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.SetCellValue("Statement", cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := file.SaveAs(filename); err != nil {
		t.Fatal(err)
	}
}
