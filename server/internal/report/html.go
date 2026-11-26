package report

import (
	"embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed templates/standard-1.html.tmpl
var templates embed.FS

var standardHTML = template.Must(template.ParseFS(templates, "templates/standard-1.html.tmpl"))

// WriteYearlyHTML renders sections prepared by PrepareYearly.
func WriteYearlyHTML(w io.Writer, metadata Template, sections []Section) error {
	if metadata.ID != StandardTemplate {
		return fmt.Errorf("unsupported yearly template %q", metadata.ID)
	}
	return standardHTML.Execute(w, struct {
		Template Template
		Sections []Section
	}{metadata, sections})
}
