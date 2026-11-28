package financials

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CompanyProfile describes the analyzed issuer so the pipeline and UI can
// generalize across banks and industries without hardcoded ticker constants.
type CompanyProfile struct {
	Ticker         string   `yaml:"ticker" json:"ticker"`
	Name           string   `yaml:"name,omitempty" json:"name,omitempty"`
	ShortName      string   `yaml:"short_name,omitempty" json:"short_name,omitempty"`
	Industry       string   `yaml:"industry,omitempty" json:"industry,omitempty"`
	Currency       string   `yaml:"currency,omitempty" json:"currency,omitempty"`
	CurrencySymbol string   `yaml:"currency_symbol,omitempty" json:"currency_symbol,omitempty"`
	ListingYear    string   `yaml:"listing_year,omitempty" json:"listing_year,omitempty"`
	CPIKey         string   `yaml:"cpi_key,omitempty" json:"cpi_key,omitempty"`
	Peers          []string `yaml:"peers,omitempty" json:"peers,omitempty"`
}

func defaultBBCAProfile() CompanyProfile {
	return CompanyProfile{
		Ticker:         "BBCA",
		Name:           "PT Bank Central Asia Tbk",
		ShortName:      "BCA",
		Industry:       "bank",
		Currency:       "IDR",
		CurrencySymbol: "Rp",
		ListingYear:    "2000",
		CPIKey:         "Indonesia CPI",
		Peers:          []string{"BBCA", "BBRI", "BMRI"},
	}
}

func readCompanyProfile(path string) (CompanyProfile, error) {
	var profile CompanyProfile
	file, err := os.Open(path)
	if err != nil {
		return profile, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&profile); err != nil {
		return profile, fmt.Errorf("parse company profile: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return profile, fmt.Errorf("company profile must contain one YAML document")
	}
	profile.Ticker = strings.ToUpper(strings.TrimSpace(profile.Ticker))
	if profile.Ticker == "" {
		return profile, fmt.Errorf("company profile requires a ticker")
	}
	if profile.ShortName == "" {
		profile.ShortName = profile.Ticker
	}
	if profile.Industry == "" {
		profile.Industry = "bank"
	}
	if profile.Currency == "" {
		profile.Currency = "IDR"
	}
	if profile.CurrencySymbol == "" {
		profile.CurrencySymbol = profile.Currency
	}
	if profile.CPIKey == "" {
		profile.CPIKey = "CPI growth"
	}
	return profile, nil
}

func (r *AnalysisReport) profile() CompanyProfile {
	if r != nil && r.Company != nil && strings.TrimSpace(r.Company.Ticker) != "" {
		return *r.Company
	}
	return defaultBBCAProfile()
}

func (r *AnalysisReport) companyTicker() string {
	return r.profile().Ticker
}

func (r *AnalysisReport) companyShortName() string {
	p := r.profile()
	if p.ShortName != "" {
		return p.ShortName
	}
	return p.Ticker
}

func (r *AnalysisReport) listingYear() string {
	p := r.profile()
	if p.ListingYear != "" {
		return p.ListingYear
	}
	return bbcaListingYear
}

func (r *AnalysisReport) cpiKey() string {
	p := r.profile()
	if p.CPIKey != "" {
		return p.CPIKey
	}
	return "Indonesia CPI"
}
