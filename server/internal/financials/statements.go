package financials

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const statementParserVersion = "statements-v4"

var (
	bcaHeading       = regexp.MustCompile(`(?i)\bpt\s+bank\s+ce\s*ntral\s+asia\b`)
	publishedOrdinal = regexp.MustCompile(`^\d+\.\s*`)
	publicationDate  = regexp.MustCompile(`(?i)^[a-z]+\s+\d{1,2},\s+(?:19|20)\d{2}$`)
	auditorHeader    = regexp.MustCompile(`(?i)independent auditor[s’']*(?:s)? report|report of independent auditors|laporan auditor independen`)
	cellGap          = regexp.MustCompile(`[ \t]{2,}|\t`)
	statementAmount  = regexp.MustCompile(`^(?:[-–—]|-?\d+(?:[.,]\d+)*\)?|\(\d+(?:[.,]\d+)*\))$`)
	yearCell         = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)
	statementTitles  = []struct{ kind, pattern string }{
		{"balance", `(?i)^(?:consolidated statements? of financial position|consolidated balance sheets?|laporan posisi keuangan konsolidasian|neraca konsolidasi)\b`},
		{"income", `(?i)^(?:consolidated statements? of (?:profit or loss|income|comprehensive income)|laporan laba rugi (?:dan penghasilan|komprehensif konsolidasian|konsolidasi)|laporan laba-rugi konsolidasi)\b`},
		{"cash", `(?i)^(?:consolidated statements? of cash flows?|laporan arus kas konsolidasi)\b`},
	}
	monthNames = [][]string{{"january", "januari"}, {"february", "februari"}, {"march", "maret"}, {"april"}, {"may", "mei"}, {"june", "juni"}, {"july", "juli"}, {"august", "agustus"}, {"september"}, {"october", "oktober"}, {"november"}, {"december", "desember"}}
)

type statementColumn struct {
	period, start, end, kind, audit, basis string
}

// Exact row labels are interpreted inside their statement, after labels, notes
// and amounts have been separated. In particular a subtotal or note heading
// cannot match a primary total merely by sharing its prefix.
var statementRows = map[string]string{
	"total assets|total asset|total aset|jumlah aset|jumlah aktiva|total aktiva": "total_assets",
	"total liabilities|jumlah liabilitas|jumlah kewajiban":                       "total_liabilities",
	"total equity|total equities|jumlah ekuitas|modal sendiri":                   "shareholders_equity",
	"total equity attributable to equity holders of parent entity|jumlah ekuitas yang dapat diatribusikan kepada pemilik entitas induk|total equity attributable to equity holders of the parent entity|diatribusikan kepada pemilik entitas induk": "common_shareholders_equity",
	"temporary syirkah deposits|dana syirkah temporer": "temporary_syirkah_deposits",
	"income before tax|profit before tax|laba sebelum pajak|laba sebelum pajak penghasilan|laba sebelum beban pajak penghasilan|profit (loss) current period before tax":                                                       "income_before_tax",
	"income tax expense|beban pajak penghasilan|jumlah beban pajak|beban pajak":                                                                                                                                                "income_tax_expense",
	"net income|net income for the year|net profit|net profit for the year|net profit (loss) current period|laba bersih|laba bersih tahun berjalan|laba tahun berjalan|laba bersih (dipindahkan)|net income (carried forward)": "net_earnings",
	"interest expense|interest expenses|beban bunga":                                                                                                             "interest_expense",
	"interest expense/return|total interest and sharia expense|jumlah beban bunga dan syariah":                                                                   "interest_and_sharia_expense",
	"total other operating expenses|jumlah beban operasional lainnya":                                                                                            "operating_expenses",
	"acquisition of fixed assets|acquisition of property and equipment|acquisition of property, plant and equipment|perolehan aset tetap|perolehan aktiva tetap": "capital_expenditures",
	"net cash provided by (used in) operating activities|net cash provided by operating activities|net cash used in operating activities|net cash from operating activities|arus kas bersih dari aktivitas operasi|kas bersih yang diperoleh dari (digunakan untuk) aktivitas operasi|kas bersih dari aktivitas operasi":                                 "net_cash_from_operating_activities",
	"net cash provided by (used in) investing activities|net cash used in investing activities|net cash provided by investing activities|net cash from investing activities|arus kas bersih dari aktivitas investasi|kas bersih untuk aktivitas investasi|kas bersih yang diperoleh dari (digunakan untuk) aktivitas investasi":                          "net_cash_from_investing_activities",
	"net cash provided by (used in) financing activities|net cash used in financing activities|net cash provided by financing activities|net cash from financing activities|arus kas bersih dari aktivitas pendanaan|kas bersih untuk aktivitas pendanaan|kas bersih yang diperoleh dari (digunakan untuk) aktivitas pendanaan":                          "net_cash_from_financing_activities",
	"weighted average number of outstanding shares|weighted average number of shares|weighted average number of ordinary shares outstanding (in full amount)|weighted average number of ordinary shares outstanding|weighted average number of ordinary shares|jumlah rata-rata tertimbang saham beredar|jumlah rata-rata tertimbang saham yang beredar": "weighted_average_common_shares_outstanding",
	"parent entity (full amount)|entitas induk (nilai penuh)": "reported_earnings_per_share",
}

var statementKeys = func() map[string]string {
	keys := make(map[string]string)
	for labels, key := range statementRows {
		for _, label := range strings.Split(labels, "|") {
			keys[label] = key
		}
	}
	return keys
}()

var compiledStatementTitles = func() []*regexp.Regexp {
	var rules []*regexp.Regexp
	for _, title := range statementTitles {
		rules = append(rules, regexp.MustCompile(title.pattern))
	}
	return rules
}()

func parseStatements(document importDocument, pages []Page) []factCandidate {
	var facts []factCandidate
	lastAuditor := -100
	for _, page := range pages {
		lines := strings.Split(page.Text, "\n")
		header := strings.Join(lines[:min(len(lines), 15)], "\n")
		lower := strings.ToLower(header)
		if auditorHeader.MatchString(header) {
			lastAuditor = page.Number
		}
		kind := statementKind(header)
		if kind == "" {
			if isEPSNote(header) {
				kind = "eps_note"
			} else {
				continue
			}
		}
		if kind != "highlights" && !bcaHeading.MatchString(header) {
			continue
		}
		audit := "unknown"
		if document.Audited || (len(document.Period) == 4 && page.Number-lastAuditor <= 25) || kind == "highlights" {
			audit = "audited"
		}
		columns, headerRow := statementColumns(lines, document.Period, kind, audit)
		if len(columns) == 0 {
			continue
		}
		currency, unit := pageUnits(header)
		if page.ExtractionMethod == "tesseract" && (strings.Contains(lower, "juteen rupiah") || strings.Contains(lower, "juts an rupiah")) {
			currency, unit = "IDR", "millions"
		}
		if kind == "highlights" {
			currency, unit = "IDR", "billions"
		}
		if kind == "eps_note" && pageBasis(header) != "consolidated" {
			continue
		}
		facts = append(facts, statementPageFacts(document, page, lines, headerRow, kind, columns, currency, unit)...)
	}
	return facts
}

func statementKind(header string) string {
	lower := strings.ToLower(header)
	if strings.Contains(lower, "entitas induk saja") || strings.Contains(lower, "parent entity only") || strings.Contains(lower, "parent entitiy only") {
		return ""
	}
	if strings.Contains(lower, "individual") && strings.Contains(lower, "accounts") {
		if strings.Contains(lower, "statements of financial position") {
			return "published_balance"
		}
		if strings.Contains(lower, "statements of profit or loss") {
			return "published_income"
		}
	}

	for _, line := range strings.Split(header, "\n") {
		for _, cell := range cellGap.Split(strings.TrimSpace(line), -1) {
			for index, rule := range compiledStatementTitles {
				if rule.MatchString(cell) {
					return statementTitles[index].kind
				}
			}
		}
	}
	if strings.Contains(lower, "key highlights in last 5 years") || strings.Contains(lower, "key financial highlights in the last 5 years") || strings.Contains(lower, "ikhtisar data keuangan 5 tahun terakhir") {
		return "highlights"
	}
	return ""
}

func isEPSNote(header string) bool {
	return strings.Contains(strings.ToLower(header), "notes to the consolidated financial statements") ||
		strings.Contains(strings.ToLower(header), "catatan atas laporan keuangan konsolidasian")
}

func statementColumns(lines []string, documentPeriod, kind, audit string) ([]statementColumn, int) {
	limit := min(len(lines), 18)
	startRow := 0
	if kind == "eps_note" {
		startRow = -1
		for i, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "basic and diluted earnings per share") || strings.Contains(lower, "laba bersih per saham dasar dan dilusian") {
				startRow = i + 1
				break
			}
		}
		if startRow < 0 {
			return nil, 0
		}
		limit = min(len(lines), startRow+18)
	}
	if strings.HasPrefix(kind, "published_") {
		return publishedColumns(lines[:limit], kind)
	}
	for lineIndex := startRow; lineIndex < limit; lineIndex++ {
		line := lines[lineIndex]
		cells := cellGap.Split(strings.TrimSpace(line), -1)
		if len(cells) == 1 {
			cells = strings.Fields(line)
		}
		var columns []statementColumn
		for _, cell := range cells {
			cell = strings.Trim(strings.TrimSpace(cell), "*)†")
			year := yearCell.FindString(cell)
			if year == "" || (cell != year && statementMonth(cell) == 0) {
				continue
			}
			month := statementMonth(cell)
			if month == 0 {
				if cell != year || (len(documentPeriod) == 7 && kind != "eps_note") {
					continue
				}
				month = 12
			}
			period := year
			if month != 12 {
				period = fmt.Sprintf("%s-%02d", year, month)
			}
			start, end, periodKind := periodBounds(period)
			columnAudit := audit
			if len(documentPeriod) == 7 {
				columnAudit = "unaudited"
				if kind == "balance" && month == 12 && strings.Contains(strings.ToLower(strings.Join(lines[:lineIndex], " ")), "(audited)") {
					columnAudit = "audited"
				}
			}
			if kind == "balance" {
				start, periodKind = "", "instant"
			}
			columns = append(columns, statementColumn{period, start, end, periodKind, columnAudit, "consolidated"})
		}
		want := 2
		if kind == "highlights" {
			want = 5
		}
		if (len(columns) == want || (kind == "balance" && len(columns) == 3)) && columns[0].period != columns[1].period && (columns[0].period == documentPeriod || columns[len(columns)-1].period == documentPeriod) {
			return columns, lineIndex
		}
	}
	return nil, 0
}

func publishedColumns(lines []string, kind string) ([]statementColumn, int) {
	for i, line := range lines {
		cells := cellGap.Split(strings.TrimSpace(line), -1)
		if i == 0 || (len(cells) != 1 && len(cells) != 4) {
			continue
		}
		var columns []statementColumn
		for j, cell := range cells {
			if !publicationDate.MatchString(cell) {
				break
			}
			year, month := yearCell.FindString(cell), statementMonth(cell)
			if month == 0 {
				break
			}
			basis, audit := "unconsolidated", "unaudited"
			if j >= 2 {
				basis = "consolidated"
			}
			period := year
			if month != 12 {
				period = fmt.Sprintf("%s-%02d", year, month)
			}
			start, end, periodKind := periodBounds(period)
			if kind == "published_balance" {
				start, periodKind = "", "instant"
				if month == 12 && strings.Contains(lines[i-1], "Audited") {
					audit = "audited"
				}
			}
			if kind == "published_income" && len(cells) == 1 {
				// The monthly publication names the end date without specifying a start date.
				start, periodKind = "", "interim_unknown_start"
			}
			columns = append(columns, statementColumn{period, start, end, periodKind, audit, basis})
		}
		if len(columns) == len(cells) {
			return columns, i
		}
	}
	return nil, 0
}

func statementMonth(cell string) int {
	text := strings.ToLower(cell)
	for index, names := range monthNames {
		for _, name := range names {
			if strings.Contains(text, name) || (len(name) >= 3 && strings.HasPrefix(text, name[:3]+" ")) {
				return index + 1
			}
		}
	}
	return 0
}

func statementPageFacts(document importDocument, page Page, lines []string, headerRow int, kind string, columns []statementColumn, currency, unit string) []factCandidate {
	var facts []factCandidate
	previousLabel := ""
	var wrapped []string
	parentIncome := false
	for index := headerRow + 1; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		label, amounts := statementCells(line, len(columns), page.ExtractionMethod == "tesseract")
		if len(amounts) == 0 {
			if strings.HasPrefix(label, "net income attributable to") || strings.HasPrefix(label, "laba bersih yang dapat") {
				parentIncome = true
			} else if strings.Contains(label, "comprehensive income attributable") || strings.HasPrefix(label, "laba komprehensif") {
				parentIncome = false
			}
			wrapped = append(wrapped, label)
			if len(wrapped) > 2 {
				wrapped = wrapped[1:]
			}
			previousLabel = label
			continue
		}
		if strings.HasPrefix(kind, "published_") {
			label = publishedOrdinal.ReplaceAllString(label, "")
		}
		key := statementKeys[label]
		if joined := statementKeys[strings.TrimSpace(previousLabel+" "+label)]; joined != "" {
			key = joined
		}
		if joined := statementKeys[strings.Join(append(wrapped, label), " ")]; joined != "" {
			key = joined
		}
		if kind == "balance" && label == "pemilik entitas induk" && strings.HasPrefix(strings.Join(wrapped, " "), "jumlah ekuitas") {
			key = "common_shareholders_equity"
		}
		if kind == "eps_note" && label == "(in full amount)" && strings.HasPrefix(strings.Join(wrapped, " "), "weighted average number of ordinary shares") {
			key = "weighted_average_common_shares_outstanding"
		}
		if parentIncome && (label == "equity holders of parent entity" || label == "equity hold ers of parent entity" || label == "equity holders of the parent entity" || label == "pemilik entitas induk") {
			key = "net_income_available_to_common_shareholders"
			parentIncome = false
		}
		previousLabel = ""
		wrapped = wrapped[:0]
		if !statementKeyAllowed(strings.TrimPrefix(kind, "published_"), key) {
			continue
		}
		for columnIndex, raw := range amounts {
			value, ok := ParseNumber(raw)
			if !ok {
				continue
			}
			column := columns[columnIndex]
			amountUnit, amountCurrency := unit, currency
			if kind == "income" && (key == "income_tax_expense" || key == "interest_expense" || key == "interest_and_sharia_expense") {
				// Primary statements present expenses as deductions. Keep benefits as negative expenses.
				value = -value
			}
			if key == "weighted_average_common_shares_outstanding" {
				amountUnit, amountCurrency = "shares", ""
			} else if key == "reported_earnings_per_share" {
				amountUnit = "per share"
			}
			if kind == "highlights" && (key == "total_assets" || key == "shareholders_equity") {
				column.start, column.kind = "", "instant"
			}
			locator := fmt.Sprintf("page:%d/%s/row:%d/column:%d", page.Number, kind, index+1, columnIndex+1)
			facts = append(facts, factCandidate{
				ID:         deterministicID(document.SHA256, page.Number, index, strings.Join([]string{statementParserVersion, locator, raw, key, column.period, column.audit, amountUnit, amountCurrency}, "|")),
				PageNumber: page.Number, SourceLocator: locator, Period: column.period, PeriodStart: column.start, PeriodEnd: column.end, PeriodType: column.kind,
				RawLabel: label, RawValue: raw, EvidenceText: line, CanonicalKey: key, NormalizedValue: strconv.FormatFloat(value, 'f', -1, 64),
				Currency: amountCurrency, Unit: amountUnit, ReportingBasis: column.basis, AuditedStatus: column.audit, ExtractionMethod: page.ExtractionMethod,
			})
		}
	}
	return facts
}

func statementKeyAllowed(kind, key string) bool {
	switch key {
	case "total_assets", "total_liabilities", "shareholders_equity", "common_shareholders_equity", "temporary_syirkah_deposits":
		return kind == "balance" || kind == "highlights"
	case "income_before_tax", "net_earnings":
		return kind == "income" || kind == "highlights"
	case "income_tax_expense", "interest_expense", "interest_and_sharia_expense", "operating_expenses", "net_income_available_to_common_shareholders", "reported_earnings_per_share":
		return kind == "income"
	case "capital_expenditures", "net_cash_from_operating_activities", "net_cash_from_investing_activities", "net_cash_from_financing_activities":
		return kind == "cash"
	case "weighted_average_common_shares_outstanding":
		return kind == "eps_note"
	}
	return false
}

func statementCells(line string, columns int, ocr bool) (string, []string) {
	cells := cellGap.Split(line, -1)
	label := cleanStatementLabel(cells[0])
	for index := 1; index < len(cells); index++ {
		end := index
		for end < len(cells) && statementAmount.MatchString(cells[end]) {
			end++
		}
		if end-index >= columns {
			return label, cells[end-columns : end]
		}
		index = end
	}
	if ocr {
		fields := strings.Fields(line)
		if len(fields) > columns {
			amounts := fields[len(fields)-columns:]
			for _, amount := range amounts {
				if !statementAmount.MatchString(amount) {
					return label, nil
				}
			}
			return cleanStatementLabel(strings.Join(fields[:len(fields)-columns], " ")), amounts
		}
	}
	return label, nil
}

func cleanStatementLabel(label string) string {
	return strings.TrimRight(strings.ToLower(strings.Join(strings.Fields(label), " ")), ":")
}

func monthPeriodBounds(period string) (string, string) {
	month, err := time.Parse("2006-01", period)
	if err != nil {
		return "", ""
	}
	return month.Format("2006") + "-01-01", month.AddDate(0, 1, -1).Format(time.DateOnly)
}
