package financials

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	reportlayout "github.com/Chandra179/lux/server/internal/report"
	"gopkg.in/yaml.v3"
)

type analysisSource struct {
	Title string `yaml:"title" json:"title"`
	URL   string `yaml:"url" json:"url"`
	Where string `yaml:"where" json:"locator"`
}

type analysisObservation struct {
	Key           string   `yaml:"key" json:"key"`
	Period        string   `yaml:"period" json:"period"`
	PeriodType    string   `yaml:"period_type,omitempty" json:"period_type,omitempty"`
	PeriodStart   string   `yaml:"period_start,omitempty" json:"period_start,omitempty"`
	PeriodEnd     string   `yaml:"period_end,omitempty" json:"period_end,omitempty"`
	Basis         string   `yaml:"basis" json:"basis"`
	Unit          string   `yaml:"unit" json:"unit"`
	Value         *float64 `yaml:"value" json:"value"`
	Source        string   `yaml:"source" json:"source"`
	Subject       string   `yaml:"subject" json:"subject"`
	Corroboration string   `yaml:"corroboration,omitempty" json:"corroboration,omitempty"`
	ShareBasis    string   `yaml:"share_basis,omitempty" json:"share_basis,omitempty"`
}

// Reviewed statement inputs cover source rows that the importer did not extract.
type analysisStatementInput struct {
	Period string             `yaml:"period"`
	Basis  string             `yaml:"basis"`
	Audit  string             `yaml:"audit"`
	Unit   string             `yaml:"unit"`
	Source string             `yaml:"source"`
	Values map[string]float64 `yaml:"values"`
}

type analysisAssessment struct {
	Status  string   `yaml:"status" json:"status"`
	Finding string   `yaml:"finding" json:"finding"`
	Gap     string   `yaml:"gap" json:"gap,omitempty"`
	Sources []string `yaml:"sources" json:"sources"`
}

type analysisEvidence struct {
	FactsHash        string                        `yaml:"facts_hash"`
	Hash             string                        `yaml:"catalog_hash"`
	Sources          map[string]analysisSource     `yaml:"sources"`
	Observations     []analysisObservation         `yaml:"observations"`
	Rules            map[string]analysisAssessment `yaml:"rules"`
	Sections         []analysisSection             `yaml:"sections"`
	HistoryNotes     map[string]string             `yaml:"history_notes"`
	StatementInputs  []analysisStatementInput      `yaml:"statement_inputs"`
	YearStories      map[string]analysisYearStory  `yaml:"year_stories"`
	YearStorySources map[string]analysisSource     `yaml:"year_story_sources"`
}

type bankInputConflict struct {
	Key     string   `yaml:"key" json:"key"`
	Period  string   `yaml:"period" json:"period"`
	Basis   string   `yaml:"basis" json:"basis"`
	Reason  string   `yaml:"reason" json:"reason"`
	Sources []string `yaml:"sources,omitempty" json:"sources,omitempty"`
}

type bankAnalysisEvidence struct {
	CatalogHash    string                        `yaml:"catalog_hash"`
	FactsHash      string                        `yaml:"facts_hash"`
	Sources        map[string]analysisSource     `yaml:"sources"`
	Observations   []analysisObservation         `yaml:"observations"`
	Rules          map[string]analysisAssessment `yaml:"rules"`
	InputConflicts []bankInputConflict           `yaml:"input_conflicts"`
}

type analysisSection struct {
	Title              string `yaml:"title" json:"title"`
	analysisAssessment `yaml:",inline"`
}

type analysisRule struct {
	Number int          `json:"number"`
	Rule   CatalogEntry `json:"rule"`
	analysisAssessment
	Metric *Metric `json:"metric,omitempty"`
}

type analysisPeriod struct {
	Period string            `json:"period"`
	Basis  string            `json:"basis"`
	Facts  map[string]Fact   `json:"facts"`
	Values map[string]Metric `json:"values"`
	Note   string            `json:"note,omitempty"`
}

type AnalysisReport struct {
	CatalogHash      string `json:"catalog_hash"`
	bankObservations []analysisObservation
	bankConflicts    []bankInputConflict
	BankScenarios    *bankScenarioAnalysis     `json:"bank_scenarios,omitempty"`
	BankCatalogHash  string                    `json:"bank_catalog_hash,omitempty"`
	FactsHash        string                    `json:"facts_hash"`
	ReviewedHash     string                    `json:"reviewed_catalog_hash"`
	Annual           []analysisPeriod          `json:"history"`
	Interim          []analysisPeriod          `json:"interim"`
	Rules            []analysisRule            `json:"rules"`
	Sections         []analysisSection         `json:"sections"`
	Sources          map[string]analysisSource `json:"sources"`
	Observations     []analysisObservation     `json:"observations"`
	Results          map[string]Metric         `json:"results"`
	BankMetrics      []Metric                  `json:"bank_metrics,omitempty"`
	InterimRisk      []analysisObservation     `json:"interim_risk"`
	PeerRows         []analysisPeerRow         `json:"peer_comparisons"`
	Coverage         analysisCoverage          `json:"evidence_coverage"`
	Yearly           []reportSection           `json:"yearly_sections,omitempty"`
	YearlyTemplate   *yearlyTemplate           `json:"yearly_template,omitempty"`
}

var historyKeys = []string{"total_assets", "common_shareholders_equity", "shareholders_equity", "net_earnings",
	"net_income_available_to_common_shareholders", "reported_earnings_per_share", "net_cash_from_operating_activities"}

func AnalysisFactsHash(ctx context.Context, db *sql.DB) (string, error) {
	facts, err := listApprovedFacts(ctx, db, "")
	if err != nil {
		return "", err
	}
	return approvedFactsHash(facts)
}

func approvedFactsHash(facts []Fact) (string, error) {
	if len(facts) == 0 {
		return "", fmt.Errorf("analysis database has no reviewed facts; see data/README.md for setup")
	}
	data, err := json.Marshal(facts)
	if err != nil {
		return "", fmt.Errorf("fingerprint approved facts: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func BuildAnalysis(ctx context.Context, db *sql.DB, catalogPath, evidencePath string) (*AnalysisReport, error) {
	return BuildAnalysisWithBankOverlay(ctx, db, catalogPath, evidencePath, "", "")
}

func BuildAnalysisWithBankOverlay(ctx context.Context, db *sql.DB, catalogPath, evidencePath, bankCatalogPath, bankEvidencePath string) (*AnalysisReport, error) {
	return BuildAnalysisWithBankScenarios(ctx, db, catalogPath, evidencePath, bankCatalogPath, bankEvidencePath, "")
}

func BuildAnalysisWithBankScenarios(ctx context.Context, db *sql.DB, catalogPath, evidencePath, bankCatalogPath, bankEvidencePath, bankScenarioPath string) (*AnalysisReport, error) {
	if bankScenarioPath != "" && (bankCatalogPath == "" || bankEvidencePath == "") {
		return nil, fmt.Errorf("bank scenarios require the bank catalog and reviewed evidence")
	}
	catalog, hash, err := LoadCatalog(catalogPath)
	if err != nil {
		return nil, err
	}
	evidence, err := readAnalysisEvidence(evidencePath)
	if err != nil {
		return nil, err
	}
	if evidence.Hash != hash {
		return nil, fmt.Errorf("analysis catalog differs from the reviewed evidence; review the changed rules before updating the evidence catalog hash")
	}
	var bankCatalog []CatalogEntry
	var bankEvidence bankAnalysisEvidence
	var bankHash string
	if (bankCatalogPath == "") != (bankEvidencePath == "") {
		return nil, fmt.Errorf("bank overlay requires both --bank-catalog and --bank-evidence")
	}
	if bankCatalogPath != "" {
		bankCatalog, bankHash, err = LoadCatalogOverlay(bankCatalogPath, catalog)
		if err != nil {
			return nil, err
		}
		if err := validateBankFormulaCatalog(bankCatalog); err != nil {
			return nil, err
		}
		bankEvidence, err = readBankAnalysisEvidence(bankEvidencePath)
		if err != nil {
			return nil, err
		}
		if bankEvidence.CatalogHash != bankHash {
			return nil, fmt.Errorf("bank catalog differs from the bank evidence; review the changed bank rules")
		}
		if err := validateBankRuleAssessments(bankCatalog, bankEvidence); err != nil {
			return nil, err
		}
	}
	for _, entry := range catalog {
		if !formulaMatches(entry) {
			return nil, fmt.Errorf("catalog rule %s differs from the registered calculation", entry.ID)
		}
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var storedHash string
	if err := tx.QueryRowContext(ctx, "SELECT catalog_hash FROM fundamental_entities LIMIT 1").Scan(&storedHash); err != nil {
		return nil, fmt.Errorf("load analysis database catalog: %w", err)
	}
	if storedHash != hash {
		return nil, fmt.Errorf("analysis database catalog differs from the YAML; run bbca-catalog against the analysis database")
	}
	approved, err := listApprovedFacts(ctx, tx, "")
	if err != nil {
		return nil, err
	}
	factsHash, err := approvedFactsHash(approved)
	if err != nil {
		return nil, err
	}
	if evidence.FactsHash != factsHash {
		return nil, fmt.Errorf("approved facts differ from the reviewed analysis snapshot; recheck the evidence and update its facts_hash")
	}
	if bankCatalogPath != "" && bankEvidence.FactsHash != factsHash {
		return nil, fmt.Errorf("approved facts differ from the reviewed bank snapshot; recheck the bank evidence and update its facts_hash")
	}
	facts := make(map[string][]periodFact)
	for _, fact := range approved {
		if n, ok := parseCanonicalNumber(fact.NormalizedValue); ok {
			facts[fact.Period] = append(facts[fact.Period], periodFact{Fact: fact, Number: n, Sources: []Fact{fact}})
		}
	}
	for _, input := range evidence.StatementInputs {
		source := evidence.Sources[input.Source]
		for key, value := range input.Values {
			if hasApprovedInput(facts[input.Period], input.Basis, key) {
				continue
			}
			start, end, kind := factPeriodBounds(input.Period, key)
			unit, currency := input.Unit, "IDR"
			if key == "weighted_average_common_shares_outstanding" {
				unit, currency = "shares", ""
			}
			fact := Fact{CanonicalKey: key, NormalizedValue: strconv.FormatFloat(value, 'f', -1, 64),
				Period: input.Period, PeriodStart: start, PeriodEnd: end, PeriodType: kind,
				ReportingBasis: input.Basis, AuditedStatus: input.Audit, Currency: currency, Unit: unit,
				DocumentID: source.URL, DocumentPath: source.Title, SourceURL: source.URL, SourceLocator: source.Where,
				Status: "reviewed", ReviewNote: "Reviewed statement input"}
			facts[input.Period] = append(facts[input.Period], periodFact{Fact: fact, Number: value, Sources: []Fact{fact}})
		}
	}
	report := &AnalysisReport{
		CatalogHash: hash, FactsHash: factsHash, ReviewedHash: evidence.Hash, Sections: evidence.Sections,
		Sources: evidence.Sources, Observations: evidence.Observations,
	}
	if bankCatalogPath != "" {
		report.BankCatalogHash = bankHash
		for key, source := range bankEvidence.Sources {
			if existing, ok := report.Sources[key]; ok && existing != source {
				return nil, fmt.Errorf("bank evidence source %s conflicts with shared evidence", key)
			}
			report.Sources[key] = source
		}
		report.Observations = append(report.Observations, bankEvidence.Observations...)
	}
	for year := 1998; year <= 2025; year++ {
		period := strconv.Itoa(year)
		row, err := analysisPeriodFor(ctx, tx, period, "consolidated", facts, evidence.HistoryNotes[period])
		if err != nil {
			return nil, err
		}
		report.Annual = append(report.Annual, row)
	}
	for _, period := range interimPeriods(facts) {
		for _, basis := range []string{"consolidated", "unconsolidated"} {
			if !hasInterimBasis(facts[period], basis) {
				continue
			}
			row, err := analysisPeriodFor(ctx, tx, period, basis, facts, evidence.HistoryNotes[period])
			if err != nil {
				return nil, err
			}
			report.Interim = append(report.Interim, row)
		}
	}
	addAnnualObservations(report)
	addInterimGrowth(report.Interim)
	addInterimMargins(report)
	for _, item := range evidence.Observations {
		if item.Subject == "BBCA" && item.Basis == "consolidated" && item.Unit == "%" && strings.HasSuffix(item.Key, " (KM1)") {
			report.InterimRisk = append(report.InterimRisk, item)
		}
	}
	report.PeerRows = analysisPeers(evidence.Observations)

	fy25 := report.Annual[len(report.Annual)-1]
	fy24 := report.Annual[len(report.Annual)-2]
	report.Results = analysisResults(fy24, fy25, evidence.Observations, facts)
	addBankValuation(report)
	addInsurerFloatProxies(report)
	if len(evidence.Rules) != len(catalog) {
		return nil, fmt.Errorf("pinned report requires %d rule assessments; got %d", len(catalog), len(evidence.Rules))
	}
	for i, entry := range catalog {
		assessment, ok := evidence.Rules[entry.ID]
		if !ok {
			return nil, fmt.Errorf("reviewed evidence lacks assessment for %s", entry.ID)
		}
		rule := analysisRule{Number: i + 1, Rule: entry, analysisAssessment: assessment}
		if metric, exists := report.Results[entry.ID]; entry.Type == "formula" && exists {
			rule.Metric = &metric
			rule.Status = metric.Status
		}
		report.Rules = append(report.Rules, rule)
	}
	if bankCatalogPath != "" {
		report.bankObservations = bankEvidence.Observations
		report.bankConflicts = bankEvidence.InputConflicts
		for _, metric := range calculateBankMetrics(bankEvidence.Observations, bankEvidence.InputConflicts) {
			if _, exists := bankEvidence.Rules[metric.EntityID]; exists {
				report.BankMetrics = append(report.BankMetrics, metric)
			}
		}
		if bankScenarioPath != "" {
			config, err := readBankScenarios(bankScenarioPath)
			if err != nil {
				return nil, err
			}
			report.BankScenarios, err = analyzeBankScenarios(config, bankEvidence)
			if err != nil {
				return nil, err
			}
		}
		latestMetrics := latestBankMetrics(report.BankMetrics)
		for _, entry := range bankCatalog {
			assessment := bankEvidence.Rules[entry.ID]
			rule := analysisRule{Number: len(report.Rules) + 1, Rule: entry, analysisAssessment: assessment}
			if metric, ok := latestMetrics[entry.ID]; entry.Type == "formula" && ok {
				rule.Metric = &metric
				rule.Status = metric.Status
			}
			report.Rules = append(report.Rules, rule)
		}
	}
	report.Coverage, err = buildAnalysisCoverage(ctx, tx, report, evidence.StatementInputs)
	if err != nil {
		return nil, err
	}
	report.Yearly, err = report.yearlySections(evidence.YearStories, evidence.YearStorySources)
	if err != nil {
		return nil, err
	}
	if len(report.Yearly) > 0 {
		report.YearlyTemplate = &yearlyTemplate{
			ID: reportlayout.StandardTemplate, Title: "BBCA Year by Year", Footer: "BBCA fundamentals · year-by-year comparison",
		}
	}
	return report, tx.Commit()
}

func validateBankRuleAssessments(catalog []CatalogEntry, evidence bankAnalysisEvidence) error {
	if len(evidence.Rules) != len(catalog) {
		return fmt.Errorf("bank evidence requires %d rule assessments; got %d", len(catalog), len(evidence.Rules))
	}
	for _, entry := range catalog {
		if _, ok := evidence.Rules[entry.ID]; !ok {
			return fmt.Errorf("bank evidence lacks assessment for %s", entry.ID)
		}
	}
	return nil
}

func addInsurerFloatProxies(report *AnalysisReport) {
	for _, insurer := range []struct{ name, id string }{
		{"BCA Life", "bca-life"}, {"BCA Insurance", "bca-insurance"},
	} {
		find := func(key, year string) analysisObservation {
			return report.findObservation(key, insurer.name, year, "standalone", "IDR million")
		}
		for _, period := range []struct{ open, close, suffix string }{
			{"2024-01-01", "2024", "-2024"}, {"2024", "2025", ""},
		} {
			open := find("Insurance contract liabilities", period.open)
			close := find("Insurance contract liabilities", period.close)
			openRe := find("Reinsurance contract assets", period.open)
			closeRe := find("Reinsurance contract assets", period.close)
			result := find("Net insurance service result", period.close)
			if open.Value == nil || close.Value == nil || openRe.Value == nil || closeRe.Value == nil || result.Value == nil ||
				open.Source != result.Source || close.Source != result.Source || openRe.Source != result.Source || closeRe.Source != result.Source {
				continue
			}
			average := (*open.Value - *openRe.Value + *close.Value - *closeRe.Value) / 2
			if average <= 0 {
				continue
			}
			id := "insurance-float-cost-proxy-" + insurer.id
			metric := derived(id, period.close, "%", -*result.Value/average*100, map[string]any{
				"opening_liabilities_restated": open, "closing_liabilities": close, "opening_reinsurance_assets_restated": openRe,
				"closing_reinsurance_assets": closeRe, "net_insurance_service_result": result, "average_net_contract_liabilities": average,
			})
			metric.Status, metric.Basis, metric.RelatedRule = "proxy_calculated", insurer.name+" standalone", "insurance-float-cost"
			report.Results[id+period.suffix] = metric
		}
	}
}

func readAnalysisEvidence(path string) (analysisEvidence, error) {
	var evidence analysisEvidence
	file, err := os.Open(path)
	if err != nil {
		return evidence, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&evidence); err != nil {
		return evidence, fmt.Errorf("read analysis evidence: %w", err)
	}
	if extra := decoder.Decode(new(any)); extra != io.EOF {
		return evidence, fmt.Errorf("analysis evidence must contain one YAML document")
	}
	for key, source := range evidence.Sources {
		if source.Title == "" || source.Where == "" || !strings.HasPrefix(source.URL, "https://") {
			return evidence, fmt.Errorf("incomplete source %s", key)
		}
	}
	validate := func(name string, entry analysisAssessment) error {
		if entry.Status == "" || entry.Finding == "" || len(entry.Sources) == 0 {
			return fmt.Errorf("incomplete assessment %s", name)
		}
		for _, source := range entry.Sources {
			if _, ok := evidence.Sources[source]; !ok {
				return fmt.Errorf("%s references unknown source %s", name, source)
			}
		}
		return nil
	}
	for id, entry := range evidence.Rules {
		if err := validate(id, entry); err != nil {
			return evidence, err
		}
	}
	for _, section := range evidence.Sections {
		if err := validate(section.Title, section.analysisAssessment); err != nil {
			return evidence, err
		}
	}
	seen := make(map[string]bool)
	for _, item := range evidence.Observations {
		key := strings.Join([]string{item.Key, item.Subject, item.Period, item.Basis}, "/")
		if item.Corroboration != "" {
			if _, ok := evidence.Sources[item.Corroboration]; !ok {
				return evidence, fmt.Errorf("observation %s references unknown corroboration %s", key, item.Corroboration)
			}
		}
		if _, ok := evidence.Sources[item.Source]; !ok || seen[key] || item.Value == nil ||
			math.IsNaN(*item.Value) || math.IsInf(*item.Value, 0) || item.Key == "" ||
			item.Period == "" || item.Basis == "" || item.Unit == "" || item.Subject == "" {
			return evidence, fmt.Errorf("duplicate or invalid observation %s", key)
		}
		seen[key] = true
	}
	for _, input := range evidence.StatementInputs {
		_, _, kind := periodBounds(input.Period)
		if (kind != "annual" && kind != "interim") || !isMoneyScale(input.Unit) ||
			(input.Basis != "consolidated" && input.Basis != "unconsolidated") ||
			(input.Audit != "audited" && input.Audit != "unaudited") || len(input.Values) == 0 {
			return evidence, fmt.Errorf("invalid reviewed statement inputs for %s", input.Period)
		}
		if _, ok := evidence.Sources[input.Source]; !ok {
			return evidence, fmt.Errorf("unknown statement input source %s", input.Source)
		}
		for key, value := range input.Values {
			scope := strings.Join([]string{"statement", input.Period, input.Basis, key}, "/")
			if !knownFactKey(key) || seen[scope] || math.IsNaN(value) || math.IsInf(value, 0) {
				return evidence, fmt.Errorf("duplicate or invalid reviewed input %s", scope)
			}
			seen[scope] = true
		}
	}
	return evidence, nil
}

func hasApprovedInput(facts []periodFact, basis, key string) bool {
	for _, item := range facts {
		if item.Fact.ReportingBasis == basis && item.Fact.CanonicalKey == key {
			return true
		}
	}
	return false
}

func interimPeriods(facts map[string][]periodFact) []string {
	var periods []string
	for period := range facts {
		if _, _, kind := periodBounds(period); kind == "interim" {
			periods = append(periods, period)
		}
	}
	sort.Strings(periods)
	return periods
}

func hasInterimBasis(facts []periodFact, basis string) bool {
	for _, fact := range facts {
		if fact.Fact.ReportingBasis == basis {
			return true
		}
	}
	return false
}

func addInterimGrowth(rows []analysisPeriod) {
	earnings := make(map[[2]string]periodFact)
	for _, row := range rows {
		fact := row.Facts["net_earnings"]
		if value, ok := parseCanonicalNumber(fact.NormalizedValue); ok && fact.PeriodType == "interim" && samePeriodFlow(fact) {
			earnings[[2]string{row.Period, row.Basis}] = periodFact{Fact: fact, Number: value}
		}
	}
	const id = "interim-net-earnings-yoy"
	for _, row := range rows {
		year, _ := strconv.Atoi(row.Period[:4])
		priorPeriod := strconv.Itoa(year-1) + row.Period[4:]
		current, currentOK := earnings[[2]string{row.Period, row.Basis}]
		prior, priorOK := earnings[[2]string{priorPeriod, row.Basis}]
		inputs := map[string]periodFact{"current": current, "prior": prior}
		if currentOK && priorOK && prior.Number > 0 &&
			metricInputCompatibility(inputs, []string{"current", "prior"}, id) == "" {
			value := (current.Number/numberAtUnit(prior, current.Fact.Unit) - 1) * 100
			row.Values[id] = derived(id, row.Period, "%", value, map[string]any{"current": current.Fact, "prior": prior.Fact})
		}
	}
}

func analysisPeriodFor(ctx context.Context, tx *sql.Tx, period, basis string, facts map[string][]periodFact, note string) (analysisPeriod, error) {
	row := analysisPeriod{Period: period, Basis: basis, Facts: make(map[string]Fact), Values: make(map[string]Metric), Note: note}
	load := func(wanted string) ([]periodFact, error) {
		var byBasis []periodFact
		for _, fact := range facts[wanted] {
			if fact.Fact.ReportingBasis == basis {
				byBasis = append(byBasis, fact)
			}
		}
		return byBasis, nil
	}
	byBasis, _ := load(period)
	selected, conflicts := numericInputs(byBasis)
	for _, key := range historyKeys {
		if item, ok := selected[key]; ok && !conflicts[key] && item.Fact.ReportingBasis == basis {
			row.Facts[key] = item.Fact
		} else if conflicts[key] {
			row.Note = strings.TrimSpace(row.Note + " Conflicting approved " + key + ".")
		}
	}
	metrics, err := calculateMetricsFrom(ctx, tx, period, load)
	if err != nil {
		return row, err
	}
	for _, metric := range metrics {
		row.Values[metric.EntityID] = metric
	}
	return row, nil
}

type analysisPeerRow struct {
	Key          string                `json:"key"`
	Basis        string                `json:"basis"`
	Status       string                `json:"status"`
	Observations []analysisObservation `json:"observations"`
}

func analysisPeers(items []analysisObservation) []analysisPeerRow {
	var rows []analysisPeerRow
	for _, key := range []string{"Gross NPL", "Capital adequacy ratio", "Loan to deposit ratio"} {
		row := analysisPeerRow{Key: key, Basis: "bank only"}
		for _, subject := range []string{"BBCA", "BBRI", "BMRI"} {
			for _, item := range items {
				if item.Period == "2025" && item.Key == key && item.Subject == subject && item.Basis == row.Basis && item.Unit == "%" {
					row.Observations = append(row.Observations, item)
				}
			}
		}
		row.Status = "comparable"
		if len(row.Observations) != 3 {
			row.Status = "matching basis or input missing"
		}
		rows = append(rows, row)
	}
	return rows
}

func derived(id, period, unit string, value float64, inputs map[string]any) Metric {
	return Metric{EntityID: id, Period: period, Unit: unit, Value: &value, Inputs: inputs, Status: "calculated"}
}

func analysisResults(fy24, fy25 analysisPeriod, items []analysisObservation, facts map[string][]periodFact) map[string]Metric {
	results := make(map[string]Metric)
	for _, rule := range []string{"earnings-per-share", "return-on-assets", "return-on-equity", "effective-tax-rate",
		"total-liabilities-to-equity", "net-change-in-cash", "capital-expenditure-to-net-earnings", "net-profit-margin"} {
		if metric, ok := fy25.Values[rule]; ok {
			results[rule] = metric
		}
	}
	obs := func(key, period, basis, unit string) (float64, any, bool) {
		for _, item := range items {
			if item.Subject == "BBCA" && item.Period == period && item.Key == key && item.Basis == basis && item.Unit == unit {
				return *item.Value, item, true
			}
		}
		return 0, nil, false
	}
	for id, metric := range annualMarketMetrics(&AnalysisReport{Observations: items}, fy25, fy24.Period) {
		results[id] = metric
	}
	for _, row := range []analysisPeriod{fy24, fy25} {
		period := row.Period
		nii, niiRef, niiOK := obs("Net interest and sharia income", period, "consolidated", "IDR million")
		other, otherRef, otherOK := obs("Other operating income", period, "consolidated", "IDR million")
		net, netOK := row.Facts["net_earnings"]
		netNumber, numberOK := parseCanonicalNumber(net.NormalizedValue)
		if niiOK && otherOK && netOK && numberOK && net.Unit == "millions" && nii+other > 0 {
			id := "bank-net-income-to-operating-income"
			results[id+"-"+period] = derived(id, period, "%", netNumber/(nii+other)*100,
				map[string]any{"net_income": net, "net_interest_and_sharia_income": niiRef, "other_operating_income": otherRef})
		}
		start, startRef, startOK := obs("Opening cash equivalents", period, "consolidated", "IDR million")
		fx, fxRef, fxOK := obs("FX effect on cash", period, "consolidated", "IDR million")
		end, endRef, endOK := obs("Closing cash equivalents", period, "consolidated", "IDR million")
		cash := row.Values["net-change-in-cash"]
		if startOK && fxOK && endOK && cash.Value != nil && cash.Unit == "IDR millions" {
			id := "cash-reconciliation-residual"
			results[id+"-"+period] = derived(id, period, "IDR million", start+*cash.Value+fx-end,
				map[string]any{"opening": startRef, "cash_subtotal": cash, "fx_effect": fxRef, "closing": endRef})
		}
		selected, conflicts := numericInputs(facts[period])
		if pbt, ok := selected["income_before_tax"]; ok && !conflicts["income_before_tax"] && pbt.Fact.Unit == "millions" && pbt.Fact.ReportingBasis == "consolidated" {
			insurance, insRef, insOK := obs("Net insurance income", period, "consolidated", "IDR million")
			provision, provisionRef, provisionOK := obs("Impairment losses", period, "consolidated", "IDR million")
			expense, expOK := selected["operating_expenses"]
			if niiOK && otherOK && insOK && provisionOK && expOK && !conflicts["operating_expenses"] &&
				expense.Fact.Unit == "millions" && expense.Fact.ReportingBasis == pbt.Fact.ReportingBasis {
				id := "pretax-bridge-residual"
				results[id+"-"+period] = derived(id, period, "IDR million", nii+other+insurance-provision+expense.Number-pbt.Number,
					map[string]any{"nii": niiRef, "other_income": otherRef, "insurance": insRef, "impairment": provisionRef, "expenses": expense.Fact, "pretax": pbt.Fact})
			}
		}
	}
	return results
}

func displayKey(s string) string {
	return strings.ReplaceAll(s, "-", " ")
}
