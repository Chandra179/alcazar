package financials

type CatalogEntry struct {
	ID          string   `yaml:"id" json:"id"`
	Type        string   `yaml:"type" json:"type"`
	Description string   `yaml:"description" json:"description"`
	Formula     string   `yaml:"formula" json:"formula,omitempty"`
	AppliesWhen string   `yaml:"applies_when" json:"applies_when,omitempty"`
	DependsOn   []string `yaml:"depends_on" json:"depends_on,omitempty"`
	Source      string   `yaml:"source" json:"source,omitempty"`
}

type Document struct {
	ID               string `json:"id"`
	SHA256           string `json:"sha256"`
	LocalPath        string `json:"local_path"`
	SourceURL        string `json:"source_url,omitempty"`
	Period           string `json:"period,omitempty"`
	DocumentType     string `json:"document_type,omitempty"`
	Format           string `json:"format"`
	Audited          bool   `json:"audited"`
	ExtractionStatus string `json:"extraction_status,omitempty"`
	PageCount        int    `json:"page_count,omitempty"`
	FactCount        int    `json:"fact_count,omitempty"`
}

type Page struct {
	Number           int
	Text             string
	ExtractionMethod string
}

type Fact struct {
	ID               string `json:"id"`
	DocumentID       string `json:"document_id"`
	DocumentPath     string `json:"document_path"`
	SourceURL        string `json:"source_url,omitempty"`
	PageNumber       int    `json:"page_number"`
	Period           string `json:"period,omitempty"`
	PeriodStart      string `json:"period_start,omitempty"`
	PeriodEnd        string `json:"period_end,omitempty"`
	PeriodType       string `json:"period_type,omitempty"`
	RawLabel         string `json:"raw_label"`
	RawValue         string `json:"raw_value"`
	EvidenceText     string `json:"evidence_text"`
	CanonicalKey     string `json:"canonical_key,omitempty"`
	NormalizedValue  string `json:"normalized_value,omitempty"`
	Currency         string `json:"currency,omitempty"`
	Unit             string `json:"unit,omitempty"`
	ReportingBasis   string `json:"reporting_basis"`
	AuditedStatus    string `json:"audited_status"`
	ExtractionMethod string `json:"extraction_method"`
	Status           string `json:"status"`
	ReviewNote       string `json:"review_note,omitempty"`
}

type Metric struct {
	EntityID string         `json:"entity_id"`
	Period   string         `json:"period"`
	Value    *float64       `json:"value,omitempty"`
	Unit     string         `json:"unit,omitempty"`
	Status   string         `json:"status"`
	Reason   string         `json:"reason,omitempty"`
	Inputs   map[string]any `json:"inputs,omitempty"`
}

type EvidenceRef struct {
	DocumentID   string `json:"document_id"`
	DocumentPath string `json:"document_path"`
	SourceURL    string `json:"source_url,omitempty"`
	PageNumber   int    `json:"page_number"`
	FactID       string `json:"fact_id,omitempty"`
	Text         string `json:"text,omitempty"`
}

type ReviewPrompt struct {
	Entity   CatalogEntry  `json:"entity"`
	Period   string        `json:"period,omitempty"`
	Evidence []EvidenceRef `json:"evidence"`
}

type CoverageGap struct {
	ID          string `json:"id"`
	Period      string `json:"period"`
	Description string `json:"description"`
	SourceURL   string `json:"source_url,omitempty"`
	Status      string `json:"status"`
}

type ImportSummary struct {
	Documents int `json:"documents"`
	Pages     int `json:"pages"`
	Facts     int `json:"facts"`
	Skipped   int `json:"skipped"`
}

type sourceMetadata struct {
	Period       string
	DocumentType string
	SourceURL    string
	Format       string
	Audited      bool
}

type importDocument struct {
	Document
	Metadata sourceMetadata
}
