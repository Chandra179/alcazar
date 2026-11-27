// Package report owns company-independent financial report layout and rendering.
package report

import (
	"html/template"
	"slices"
)

const StandardTemplate = "standard-1"

type Template struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Footer string `json:"footer"`
}

type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type Cell struct {
	Text  string `json:"text"`
	Links []Link `json:"links,omitempty"`
	Label string `json:"label,omitempty"`
}

type Table struct {
	Title          string       `json:"title,omitempty"`
	Headers        []string     `json:"headers"`
	Rows           [][]Cell     `json:"rows"`
	NumericColumns []int        `json:"numeric_columns,omitempty"`
	CurrentColumn  int          `json:"current_column,omitempty"`
	Layout         *TableLayout `json:"layout,omitempty"`
}

type ChartLegend struct {
	Label string `json:"label"`
	Color string `json:"color"`
	Kind  string `json:"kind,omitempty"`
}

type ChartBlock struct {
	Tag      string        `json:"tag,omitempty"`
	Title    string        `json:"title,omitempty"`
	Subtitle string        `json:"subtitle,omitempty"`
	Legend   []ChartLegend `json:"legend,omitempty"`
	Body     template.HTML `json:"-"`
	Note     string        `json:"note,omitempty"`
}

type Section struct {
	ID          string         `json:"id,omitempty"`
	Title       string         `json:"title,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	PeriodLabel string         `json:"period_label,omitempty"`
	Paragraphs  []Cell         `json:"paragraphs,omitempty"`
	Charts      []ChartBlock   `json:"charts,omitempty"`
	Tables      []Table        `json:"tables,omitempty"`
	Layout      *SectionLayout `json:"layout,omitempty"`
}

type SectionLayout struct {
	Takeaways  bool  `json:"takeaways,omitempty"`
	SourceRows []int `json:"source_rows,omitempty"`
}

type TableLayout struct {
	Contents    bool     `json:"contents,omitempty"`
	ColumnRoles []string `json:"column_roles"`
	GroupRows   []int    `json:"group_rows,omitempty"`
}

func (s Section) Body() []Cell {
	if s.Layout == nil || len(s.Layout.SourceRows) == 0 {
		return s.Paragraphs
	}
	var body []Cell
	for index, cell := range s.Paragraphs {
		if !slices.Contains(s.Layout.SourceRows, index) {
			body = append(body, cell)
		}
	}
	return body
}

func (s Section) Sources() []Cell {
	if s.Layout == nil {
		return nil
	}
	sources := make([]Cell, 0, len(s.Layout.SourceRows))
	for _, index := range s.Layout.SourceRows {
		sources = append(sources, s.Paragraphs[index])
	}
	return sources
}

func (t Table) ColumnRole(index int) string {
	if t.Layout == nil || index >= len(t.Layout.ColumnRoles) {
		return ""
	}
	return t.Layout.ColumnRoles[index]
}

func (t Table) Group(index int, row []Cell) bool {
	if t.Layout != nil {
		return slices.Contains(t.Layout.GroupRows, index)
	}
	return groupRow(row)
}

func groupRow(row []Cell) bool {
	if len(row) == 0 || row[0].Text == "" {
		return false
	}
	for _, cell := range row[1:] {
		if cell.Text != "" || len(cell.Links) > 0 {
			return false
		}
	}
	return true
}
