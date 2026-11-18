package financials

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Chandra179/lux/server/store"
)

type ImportOptions struct {
	DataDir      string
	CatalogPath  string
	ExtractedDir string
	OCRLanguage  string
	MinPageChars int
	SkipOCR      bool
	Force        bool
}

type factCandidate struct {
	ID               string
	PageNumber       int
	SourceLocator    string
	Period           string
	PeriodStart      string
	PeriodEnd        string
	PeriodType       string
	RawLabel         string
	RawValue         string
	EvidenceText     string
	CanonicalKey     string
	NormalizedValue  string
	Currency         string
	Unit             string
	ReportingBasis   string
	AuditedStatus    string
	ExtractionMethod string
}

type labelAlias struct {
	key string
	re  *regexp.Regexp
}

var financialAliases = []labelAlias{
	{key: "income_before_tax", re: regexp.MustCompile(`(?i)income before tax|pre-tax (?:profit|income)|laba sebelum pajak|laba sebelum beban pajak`)},
	{key: "net_income_available_to_common_shareholders", re: regexp.MustCompile(`(?i)net income available to common|net earnings attributable to (?:owners|common)|laba bersih yang dapat diatribusikan kepada pemilik`)},
	{key: "net_earnings", re: regexp.MustCompile(`(?i)net profit|net income|laba bersih|laba tahun berjalan`)},
	{key: "income_tax_expense", re: regexp.MustCompile(`(?i)income tax expense|tax expense|beban pajak penghasilan`)},
	{key: "total_liabilities", re: regexp.MustCompile(`(?i)total liabilities|jumlah liabilitas|jumlah kewajiban|total kewajiban`)},
	{key: "shareholders_equity", re: regexp.MustCompile(`(?i)total equity|shareholders.? equity|total shareholders.? equity|jumlah ekuitas|modal sendiri|ekuitas yang dapat diatribusikan`)},
	{key: "total_assets", re: regexp.MustCompile(`(?i)total assets|jumlah aset|total aktiva|jumlah aktiva`)},
	{key: "current_liabilities", re: regexp.MustCompile(`(?i)current liabilities|liabilitas jangka pendek|kewajiban jangka pendek`)},
	{key: "current_assets", re: regexp.MustCompile(`(?i)current assets|aset lancar|aktiva lancar`)},
	{key: "weighted_average_common_shares_outstanding", re: regexp.MustCompile(`(?i)weighted average.*shares|weighted average number of shares|rata-rata tertimbang jumlah saham`)},
	{key: "selling_general_and_administrative_expense", re: regexp.MustCompile(`(?i)selling,? general and administrative|beban umum dan administrasi`)},
	{key: "research_and_development_expense", re: regexp.MustCompile(`(?i)research and development expense|beban penelitian dan pengembangan`)},
	{key: "depreciation_expense", re: regexp.MustCompile(`(?i)depreciation expense|beban penyusutan`)},
	{key: "capital_expenditures", re: regexp.MustCompile(`(?i)capital expenditures|acquisition of (?:property.*equipment|fixed assets)|perolehan aset tetap`)},
	{key: "net_cash_from_operating_activities", re: regexp.MustCompile(`(?i)net cash (?:provided by|from) operating activities|arus kas bersih dari aktivitas operasi`)},
	{key: "net_cash_from_investing_activities", re: regexp.MustCompile(`(?i)net cash (?:used in|from) investing activities|arus kas bersih dari aktivitas investasi`)},
	{key: "net_cash_from_financing_activities", re: regexp.MustCompile(`(?i)net cash (?:used in|from) financing activities|arus kas bersih dari aktivitas pendanaan`)},
	{key: "interest_expense", re: regexp.MustCompile(`(?i)interest expense|beban bunga`)},
	{key: "operating_expenses", re: regexp.MustCompile(`(?i)operating expenses|operating costs|beban operasional`)},
	{key: "cost_of_revenue", re: regexp.MustCompile(`(?i)cost of revenue|cost of goods sold|beban pokok pendapatan`)},
	{key: "revenue", re: regexp.MustCompile(`(?i)total operating income|operating income|total revenue|revenue|pendapatan operasional|pendapatan usaha`)},
	{key: "net_receivables", re: regexp.MustCompile(`(?i)net receivables|piutang usaha bersih`)},
	{key: "inventory", re: regexp.MustCompile(`(?i)^\s*(?:inventories|inventory|persediaan)\b`)},
	{key: "gross_profit", re: regexp.MustCompile(`(?i)gross profit|laba bruto`)},
}

func detectCanonicalKey(line string) string {
	label := strings.TrimLeft(strings.TrimSpace(line), "0123456789 .:()#*-\t")
	for _, alias := range financialAliases {
		match := alias.re.FindStringIndex(label)
		if match != nil && match[0] == 0 {
			return alias.key
		}
	}
	return ""
}

func Import(ctx context.Context, db *sql.DB, options ImportOptions) (ImportSummary, error) {
	var summary ImportSummary
	if strings.TrimSpace(options.DataDir) == "" {
		return summary, fmt.Errorf("data directory is required")
	}
	if strings.TrimSpace(options.CatalogPath) == "" {
		return summary, fmt.Errorf("catalog path is required")
	}
	if options.OCRLanguage == "" {
		options.OCRLanguage = "eng+ind"
	}
	if options.MinPageChars <= 0 {
		options.MinPageChars = 60
	}
	if options.ExtractedDir == "" {
		options.ExtractedDir = filepath.Join(options.DataDir, "extracted")
	}
	if err := store.EnsureBBCAFinancialSchema(ctx, db); err != nil {
		return summary, err
	}
	entries, catalogHash, err := LoadCatalog(options.CatalogPath)
	if err != nil {
		return summary, err
	}
	if err := storeCatalog(ctx, db, entries, catalogHash); err != nil {
		return summary, err
	}
	documents, err := inventoryDocuments(options.DataDir)
	if err != nil {
		return summary, err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		processed, err := registerDocument(db, document)
		if err != nil {
			return summary, err
		}
		summary.Documents++
		if processed && !options.Force {
			summary.Skipped++
			continue
		}
		pages, facts, err := extractDocument(ctx, db, document, options)
		if err != nil {
			return summary, fmt.Errorf("extract %s: %w", document.LocalPath, err)
		}
		if err := storeExtractedDocument(ctx, db, document, pages, facts); err != nil {
			return summary, err
		}
		summary.Pages += len(pages)
		summary.Facts += len(facts)
	}
	return summary, nil
}

func inventoryDocuments(dataDir string) ([]importDocument, error) {
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	metadata, err := readManifest(filepath.Join(root, "manifest.csv"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var documents []importDocument
	err = filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filename != root && (entry.Name() == "extracted" || entry.Name() == "review") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != ".pdf" && ext != ".xlsx" && ext != ".xlsm" {
			return nil
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		meta, ok := metadata[filepath.ToSlash(relative)]
		if !ok {
			meta = sourceMetadata{Format: strings.TrimPrefix(strings.ToUpper(ext), ".")}
			meta.Period = inferPeriod(filepath.Base(filename))
			meta.DocumentType = "financial report"
		}
		file, err := os.Open(filename)
		if err != nil {
			return fmt.Errorf("open %s: %w", relative, err)
		}
		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			_ = file.Close()
			return fmt.Errorf("hash %s: %w", relative, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %s: %w", relative, err)
		}
		hash := hex.EncodeToString(hasher.Sum(nil))
		doc := Document{
			ID: hash, SHA256: hash, LocalPath: filepath.ToSlash(relative), SourceURL: meta.SourceURL,
			Period: meta.Period, DocumentType: meta.DocumentType, Format: meta.Format,
			Audited: meta.Audited,
		}
		documents = append(documents, importDocument{Document: doc, Metadata: meta})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventory data directory: %w", err)
	}
	sortDocuments(documents)
	return documents, nil
}

func readManifest(filename string) (map[string]sourceMetadata, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read manifest header: %w", err)
	}
	indices := make(map[string]int, len(header))
	for index, name := range header {
		indices[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))] = index
	}
	get := func(row []string, key string) string {
		index, ok := indices[key]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	result := make(map[string]sourceMetadata)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read manifest row: %w", err)
		}
		local := filepath.ToSlash(filepath.Clean(get(row, "local_file")))
		if local == "." || local == "" {
			continue
		}
		docType, period, format := get(row, "document_type"), get(row, "period"), get(row, "format")
		result[local] = sourceMetadata{
			Period: period, DocumentType: docType, SourceURL: get(row, "direct_source_url"),
			Format: strings.ToUpper(format), Audited: strings.HasPrefix(strings.ToLower(docType), "audited"),
		}
	}
	return result, nil
}

func registerDocument(db *sql.DB, document importDocument) (bool, error) {
	var status, version string
	err := db.QueryRow(`SELECT d.extraction_status, COALESCE(p.version, '') FROM financial_documents d
		LEFT JOIN financial_document_parsers p ON p.document_id = d.id WHERE d.sha256 = ?`, document.SHA256).Scan(&status, &version)
	processed := err == nil && status == "processed" && version == statementParserVersion
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("check document %s: %w", document.LocalPath, err)
	}
	_, err = db.Exec(`INSERT INTO financial_documents
		(id, sha256, local_path, source_url, period, document_type, format, audited, extraction_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')
		ON CONFLICT(sha256) DO UPDATE SET local_path = excluded.local_path,
			source_url = excluded.source_url, period = excluded.period,
			document_type = excluded.document_type, format = excluded.format,
			audited = excluded.audited,
		extraction_status = financial_documents.extraction_status`,
		document.ID, document.SHA256, document.LocalPath, document.SourceURL,
		document.Period, document.DocumentType, document.Format, boolInt(document.Audited))
	if err != nil {
		return false, fmt.Errorf("register document %s: %w", document.LocalPath, err)
	}
	return processed, nil
}

func extractDocument(ctx context.Context, db *sql.DB, document importDocument, options ImportOptions) ([]Page, []factCandidate, error) {
	filename := filepath.Join(options.DataDir, filepath.FromSlash(document.LocalPath))
	var pages []Page
	var facts []factCandidate
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		var err error
		pages, err = loadStoredPages(ctx, db, document.ID)
		if err != nil {
			return nil, nil, err
		}
		if len(pages) == 0 {
			pages, err = extractPDF(ctx, filename, options)
			if err != nil {
				return nil, nil, err
			}
			if err := writeExtractedPages(options.ExtractedDir, document.SHA256, pages); err != nil {
				return nil, nil, err
			}
		}
		facts = parseStatements(document, pages)
	case ".xlsx", ".xlsm":
		workbookFacts, err := extractXLSX(filename)
		if err != nil {
			return nil, nil, err
		}
		for index, candidate := range workbookFacts {
			key := detectCanonicalKey(candidate.Label)
			value, valid := candidate.Number, candidate.IsNumber
			if !valid {
				value, valid = ParseNumber(candidate.Value)
			}
			if !valid {
				continue
			}
			periodStart, periodEnd, periodType := periodBounds(document.Period)
			auditedStatus := "unknown"
			if document.Audited {
				auditedStatus = "audited"
			}
			facts = append(facts, factCandidate{
				ID: deterministicID(document.SHA256, 0, index, strings.Join([]string{statementParserVersion, candidate.Sheet, candidate.Cell, candidate.Label, candidate.Value, document.Period}, "|")), PageNumber: 0,
				SourceLocator: candidate.Sheet + "!" + candidate.Cell,
				Period:        document.Period, PeriodStart: periodStart, PeriodEnd: periodEnd,
				PeriodType: periodType, RawLabel: candidate.Sheet + " / " + candidate.Label,
				RawValue: candidate.Value, EvidenceText: candidate.Evidence, CanonicalKey: key,
				NormalizedValue: strconv.FormatFloat(value, 'f', -1, 64), Currency: candidate.Currency,
				Unit: candidate.Unit, ReportingBasis: "unknown", AuditedStatus: auditedStatus,
				ExtractionMethod: "xlsx",
			})
		}
	default:
		return nil, nil, fmt.Errorf("unsupported document format %q", filepath.Ext(filename))
	}
	return pages, facts, nil
}

func loadStoredPages(ctx context.Context, db *sql.DB, documentID string) ([]Page, error) {
	rows, err := db.QueryContext(ctx, `SELECT page_number, text, extraction_method
		FROM financial_pages WHERE document_id = ? ORDER BY page_number`, documentID)
	if err != nil {
		return nil, fmt.Errorf("load cached extracted pages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pages []Page
	for rows.Next() {
		var page Page
		if err := rows.Scan(&page.Number, &page.Text, &page.ExtractionMethod); err != nil {
			return nil, fmt.Errorf("scan cached page: %w", err)
		}
		pages = append(pages, page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cached pages: %w", err)
	}
	return pages, nil
}

func extractPDF(ctx context.Context, filename string, options ImportOptions) ([]Page, error) {
	command := exec.CommandContext(ctx, "pdftotext", "-layout", filename, "-")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("run pdftotext: %w", err)
	}
	chunks := strings.Split(string(output), "\f")
	if len(chunks) > 0 && strings.TrimSpace(chunks[len(chunks)-1]) == "" {
		chunks = chunks[:len(chunks)-1]
	}
	pages := make([]Page, 0, len(chunks))
	for index, pageText := range chunks {
		method := "pdftotext"
		if !options.SkipOCR && nonSpaceCount(pageText) < options.MinPageChars {
			ocrText, ocrErr := ocrPDFPage(ctx, filename, index+1, options.OCRLanguage)
			if ocrErr != nil {
				return nil, fmt.Errorf("OCR page %d: %w", index+1, ocrErr)
			}
			if nonSpaceCount(ocrText) > nonSpaceCount(pageText) {
				pageText, method = ocrText, "tesseract"
			}
		}
		pages = append(pages, Page{Number: index + 1, Text: pageText, ExtractionMethod: method})
	}
	return pages, nil
}

func ocrPDFPage(ctx context.Context, filename string, page int, languages string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "bbca-ocr-")
	if err != nil {
		return "", fmt.Errorf("create OCR temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	prefix := filepath.Join(tmpDir, "page")
	render := exec.CommandContext(ctx, "pdftoppm", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-r", "300", "-png", "-singlefile", filename, prefix)
	if output, err := render.CombinedOutput(); err != nil {
		return "", fmt.Errorf("render PDF page: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	imagePath := prefix + ".png"
	ocr := exec.CommandContext(ctx, "tesseract", imagePath, "stdout", "-l", languages, "--psm", "6")
	output, err := ocr.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run tesseract: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func storeExtractedDocument(ctx context.Context, db *sql.DB, document importDocument, pages []Page, facts []factCandidate) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin document import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM financial_fact_sources WHERE fact_id IN
		(SELECT id FROM financial_facts WHERE document_id = ? AND status = 'pending')`, document.ID); err != nil {
		return fmt.Errorf("replace pending fact sources: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM financial_facts WHERE document_id = ? AND status = 'pending'`, document.ID); err != nil {
		return fmt.Errorf("replace pending facts: %w", err)
	}
	for _, page := range pages {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO financial_pages
			(document_id, page_number, extraction_method, text) VALUES (?, ?, ?, ?)`,
			document.ID, page.Number, page.ExtractionMethod, page.Text); err != nil {
			return fmt.Errorf("store page %d for %s: %w", page.Number, document.LocalPath, err)
		}
	}
	for _, fact := range facts {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO financial_facts
				(id, document_id, page_number, period, period_start, period_end, period_type,
			raw_label, raw_value, evidence_text, canonical_key, normalized_value, currency,
			unit, reporting_basis, audited_status, extraction_method, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
			fact.ID, document.ID, fact.PageNumber, fact.Period, fact.PeriodStart, fact.PeriodEnd,
			fact.PeriodType, fact.RawLabel, fact.RawValue, fact.EvidenceText, fact.CanonicalKey,
			fact.NormalizedValue, fact.Currency, fact.Unit, fact.ReportingBasis,
			fact.AuditedStatus, fact.ExtractionMethod); err != nil {
			return fmt.Errorf("store candidate fact %s: %w", fact.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO financial_fact_sources (fact_id, source_locator)
				VALUES (?, ?) ON CONFLICT(fact_id) DO UPDATE SET source_locator = excluded.source_locator`,
			fact.ID, fact.SourceLocator); err != nil {
			return fmt.Errorf("store source locator for fact %s: %w", fact.ID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE financial_documents SET extraction_status = 'processed' WHERE id = ?`, document.ID); err != nil {
		return fmt.Errorf("mark document %s processed: %w", document.LocalPath, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO financial_document_parsers (document_id, version) VALUES (?, ?)
		ON CONFLICT(document_id) DO UPDATE SET version = excluded.version`, document.ID, statementParserVersion); err != nil {
		return fmt.Errorf("record parser version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit document import: %w", err)
	}
	return nil
}

func writeExtractedPages(root, documentHash string, pages []Page) error {
	if len(pages) == 0 {
		return nil
	}
	dir := filepath.Join(root, documentHash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	for _, page := range pages {
		filename := filepath.Join(dir, fmt.Sprintf("page-%04d.txt", page.Number))
		contents := fmt.Sprintf("[method=%s]\n%s", page.ExtractionMethod, page.Text)
		if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
			return fmt.Errorf("write extracted page %d: %w", page.Number, err)
		}
	}
	return nil
}

var filenamePeriod = regexp.MustCompile(`(?:19|20)\d{2}(?:-(?:0[1-9]|1[0-2]))?`)

func inferPeriod(filename string) string {
	return filenamePeriod.FindString(filename)
}

func periodBounds(period string) (string, string, string) {
	if len(period) == 4 {
		if year, err := strconv.Atoi(period); err == nil && year >= 1900 && year <= 2099 {
			return period + "-01-01", period + "-12-31", "annual"
		}
	}
	if len(period) == 7 {
		if start, end := monthPeriodBounds(period); end != "" {
			return start, end, "interim"
		}
	}
	return "", "", "unknown"
}

func deterministicID(documentHash string, page, line int, token string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", documentHash, page, line, token)))
	return hex.EncodeToString(sum[:])
}

func pageUnits(text string) (string, string) {
	lower := strings.ToLower(text)
	currency := ""
	if strings.Contains(lower, "rupiah") || strings.Contains(lower, "rp ") || strings.Contains(lower, "rp.") {
		currency = "IDR"
	}
	unit := ""
	switch {
	case strings.Contains(lower, "in millions") || strings.Contains(lower, "dalam jutaan"):
		unit = "millions"
	case strings.Contains(lower, "in billions") || strings.Contains(lower, "dalam miliar"):
		unit = "billions"
	case strings.Contains(lower, "in thousand") || strings.Contains(lower, "dalam ribuan"):
		unit = "thousands"
	}
	return currency, unit
}

func pageBasis(text string) string {
	lower := strings.ToLower(text)
	unconsolidated := strings.Contains(lower, "unconsolidated") || strings.Contains(lower, "tidak konsolidasian")
	consolidated := strings.Contains(lower, "consolidated") || strings.Contains(lower, "konsolidasian")
	if unconsolidated && consolidated {
		return "unknown"
	}
	if unconsolidated {
		return "unconsolidated"
	}
	if consolidated {
		return "consolidated"
	}
	return "unknown"
}

func nonSpaceCount(text string) int {
	count := 0
	for _, r := range text {
		if r > ' ' {
			count++
		}
	}
	return count
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func sortDocuments(documents []importDocument) {
	for i := 1; i < len(documents); i++ {
		for j := i; j > 0 && documents[j].LocalPath < documents[j-1].LocalPath; j-- {
			documents[j], documents[j-1] = documents[j-1], documents[j]
		}
	}
}
