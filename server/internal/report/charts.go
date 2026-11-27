package report

import (
	"fmt"
	"html"
	"html/template"
	"math"
	"slices"
	"strconv"
	"strings"
)

type annualPoint struct {
	Year        int
	Label       string
	Profit      *float64
	ProfitUnit  string
	ROE         *float64
	ROA         *float64
	DivYield    *float64
	PE          *float64
	PBV         *float64
	TotalReturn *float64
}

// YearlyChartSections builds two visual overview sections (growth/compounding and ratios/returns)
// ordered chronologically from oldest to newest annual record.
func YearlyChartSections(sections []Section) []Section {
	points := extractAnnualSeries(sections)
	if len(points) < 4 {
		return nil
	}
	// Find first year with positive profit and core ratios so pre-reorg/crisis gaps don't break log/ratio scales.
	start := 0
	for start < len(points) && (points[start].Profit == nil || *points[start].Profit <= 0 || points[start].ROE == nil) {
		start++
	}
	if len(points)-start < 4 {
		return nil
	}
	series := points[start:]
	periodSpan := fmt.Sprintf("%s–%s", series[0].Label, series[len(series)-1].Label)
	skippedNote := ""
	if start > 0 {
		skippedNote = fmt.Sprintf("Note: Pre-reorganization / crisis years (%s–%s) are omitted from the %s scale.", points[0].Label, points[start-1].Label, periodSpan)
	}

	unit := "T"
	for _, p := range series {
		if p.ProfitUnit != "" {
			unit = p.ProfitUnit
			break
		}
	}

	page1 := Section{
		ID:          "yearly-charts-growth",
		Kind:        "charts",
		Title:       "Long-term growth and compounding",
		PeriodLabel: periodSpan,
		Charts: []ChartBlock{
			{
				Tag:      "Scale vs. Profitability",
				Title:    fmt.Sprintf("Group profit (%s) & Return on equity (%s)", unit, periodSpan),
				Subtitle: "Muted bars show earnings scale; teal line tracks capital efficiency (%).",
				Legend: []ChartLegend{
					{Label: fmt.Sprintf("Group profit (%s)", unit), Color: "#cbd5e1", Kind: "bar"},
					{Label: "ROE (%)", Color: "#0f766e", Kind: "line"},
				},
				Body: template.HTML(renderProfitROEComboSVG(series, unit)),
				Note: skippedNote,
			},
			{
				Tag:      "Multi-Decade Compounding",
				Title:    fmt.Sprintf("Indexed growth on logarithmic scale (%s = 100)", series[0].Label),
				Subtitle: "Log scale compares early compounding pace equally alongside recent years.",
				Legend: []ChartLegend{
					{Label: "Cumulative total return (incl. dividends)", Color: "#0f766e", Kind: "line"},
					{Label: "Group profit index", Color: "#334155", Kind: "line"},
				},
				Body: template.HTML(renderIndexedLogSVG(series)),
			},
		},
	}

	page2 := Section{
		ID:          "yearly-charts-outcomes",
		Kind:        "charts",
		Title:       "Long-term ratios and shareholder returns",
		PeriodLabel: periodSpan,
		Charts: []ChartBlock{
			{
				Tag:      "Profitability & Valuation Cycles",
				Title:    fmt.Sprintf("Key ratios & multiples (%s)", periodSpan),
				Subtitle: "One series per panel with dashed historical median and latest endpoint.",
				Legend: []ChartLegend{
					{Label: "Annual value", Color: "#0f766e", Kind: "line"},
					{Label: "Period median", Color: "#94a3b8", Kind: "dashed"},
				},
				Body: template.HTML(renderSmallMultiplesHTML(series)),
			},
			{
				Tag:      "Annual Shareholder Outcome",
				Title:    fmt.Sprintf("Annual total return including cash dividends (%s–%s)", returnStartLabel(series), series[len(series)-1].Label),
				Subtitle: "Anchored at 0% so positive compounding years and drawdown years stand out.",
				Legend: []ChartLegend{
					{Label: "Positive year", Color: "#0f766e", Kind: "bar"},
					{Label: "Negative year", Color: "#be123c", Kind: "bar"},
				},
				Body: template.HTML(renderDivergingReturnSVG(series)),
			},
		},
	}

	return []Section{page1, page2}
}

func returnStartLabel(series []annualPoint) string {
	for _, p := range series {
		if p.TotalReturn != nil {
			return p.Label
		}
	}
	return series[0].Label
}

func extractAnnualSeries(sections []Section) []annualPoint {
	var annuals []Section
	for _, s := range sections {
		if s.Kind == "annual" {
			annuals = append(annuals, s)
		}
	}
	// Annual sections are ordered newest -> oldest; reverse to oldest -> newest.
	slices.Reverse(annuals)
	out := make([]annualPoint, 0, len(annuals))
	for _, s := range annuals {
		yr, err := strconv.Atoi(strings.TrimSpace(s.PeriodLabel))
		if err != nil {
			continue
		}
		pt := annualPoint{Year: yr, Label: s.PeriodLabel}
		for _, tbl := range s.Tables {
			col := tbl.CurrentColumn
			if col <= 0 && len(tbl.Headers) > 0 {
				col = len(tbl.Headers) - 1
			}
			for _, row := range tbl.Rows {
				if len(row) <= col || groupRow(row) {
					continue
				}
				name := strings.TrimSpace(row[0].Text)
				raw := strings.TrimSpace(row[col].Text)
				val, unit, ok := parseMetricValue(raw)
				if !ok {
					continue
				}
				switch name {
				case "Group profit":
					pt.Profit = &val
					pt.ProfitUnit = unit
				case "Return on equity":
					pt.ROE = &val
				case "Return on assets":
					pt.ROA = &val
				case "Dividend yield":
					pt.DivYield = &val
				case "Price / earnings":
					pt.PE = &val
				case "Price / book value":
					pt.PBV = &val
				case "Return including cash dividends":
					pt.TotalReturn = &val
				}
			}
		}
		out = append(out, pt)
	}
	return out
}

func parseMetricValue(raw string) (float64, string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "—" || s == "-" || strings.EqualFold(s, "level only") {
		return 0, "", false
	}
	s = strings.ReplaceAll(s, ",", "")
	unit := ""
	for _, suffix := range []string{"T", "B", "M", "%", "×", "x"} {
		if strings.HasSuffix(s, suffix) {
			unit = suffix
			s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
			break
		}
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, "", false
	}
	return val, unit, true
}

func niceMax(maxVal float64, steps int) float64 {
	if maxVal <= 0 {
		return 10
	}
	rawStep := maxVal / float64(steps)
	pow := math.Pow(10, math.Floor(math.Log10(rawStep)))
	norm := rawStep / pow
	var step float64
	switch {
	case norm <= 1:
		step = 1 * pow
	case norm <= 2:
		step = 2 * pow
	case norm <= 2.5:
		step = 2.5 * pow
	case norm <= 5:
		step = 5 * pow
	default:
		step = 10 * pow
	}
	return math.Ceil(maxVal/step) * step
}

func renderProfitROEComboSVG(series []annualPoint, unit string) string {
	const W, H = 820.0, 210.0
	const L, R, T, B = 38.0, 40.0, 18.0, 22.0
	pw, ph := W-L-R, H-T-B

	maxProf, maxROE := 0.0, 0.0
	for _, p := range series {
		if p.Profit != nil && *p.Profit > maxProf {
			maxProf = *p.Profit
		}
		if p.ROE != nil && *p.ROE > maxROE {
			maxROE = *p.ROE
		}
	}
	profTop := niceMax(maxProf*1.08, 3)
	roeTop := niceMax(maxROE*1.12, 3)

	n := len(series)
	xAt := func(i int) float64 { return L + (float64(i)+0.5)*(pw/float64(n)) }
	bw := (pw / float64(n)) * 0.62
	yProf := func(v float64) float64 { return T + ph - (v/profTop)*ph }
	yROE := func(v float64) float64 { return T + ph - (v/roeTop)*ph }

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg">`, W, H)

	// Macro stress bands if present (2008 GFC, 2020 COVID)
	for i, p := range series {
		if p.Year == 2008 || p.Year == 2020 {
			label := "GFC"
			if p.Year == 2020 {
				label = "COVID"
			}
			bx := xAt(i) - bw*0.85
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#f1f5f9"/>`, bx, T, bw*1.7, ph)
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="8" font-weight="600" fill="#94a3b8">%s</text>`, xAt(i), T+9, label)
		}
	}

	// Gridlines & left axis (Profit)
	for k := 0; k <= 3; k++ {
		v := profTop * float64(k) / 3.0
		y := yProf(v)
		stroke, sw := "#e5e7eb", "0.75"
		if k == 0 {
			stroke, sw = "#9ca3af", "1"
		}
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%s"/>`, L, y, W-R, y, stroke, sw)
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="end" font-size="9" fill="#6b7280">%.0f%s</text>`, L-5, y+3, v, html.EscapeString(unit))
	}
	// Right axis (ROE)
	for k := 0; k <= 3; k++ {
		v := roeTop * float64(k) / 3.0
		y := yROE(v)
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="9" fill="#0f766e">%.0f%%</text>`, W-R+5, y+3, v)
	}

	// Bars
	for i, p := range series {
		if p.Profit == nil || *p.Profit < 0 {
			continue
		}
		y := yProf(*p.Profit)
		fill := "#cbd5e1"
		if i == n-1 {
			fill = "#94a3b8"
		}
		fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="1.5" fill="%s"/>`, xAt(i)-bw/2, y, bw, yProf(0)-y, fill)
	}

	// ROE line
	var pts []string
	for i, p := range series {
		if p.ROE != nil {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", xAt(i), yROE(*p.ROE)))
		}
	}
	if len(pts) > 1 {
		fmt.Fprintf(&sb, `<polyline points="%s" fill="none" stroke="#0f766e" stroke-width="2.1" stroke-linejoin="round"/>`, strings.Join(pts, " "))
	}

	// X ticks & endpoint labels
	for i, p := range series {
		if p.Year%5 == 0 || i == 0 || i == n-1 {
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="9" fill="#6b7280">%d</text>`, xAt(i), H-5, p.Year)
		}
	}
	last := series[n-1]
	lastX := xAt(n - 1)
	if last.Profit != nil {
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="9" font-weight="700" fill="#475569">%.1f%s</text>`, lastX, yProf(*last.Profit)-4, *last.Profit, html.EscapeString(unit))
	}
	if last.ROE != nil {
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="3" fill="#0f766e" stroke="#fff" stroke-width="1.2"/>`, lastX, yROE(*last.ROE))
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="end" font-size="9" font-weight="700" fill="#0f766e">%.1f%%</text>`, lastX-4, yROE(*last.ROE)-6, *last.ROE)
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

func renderIndexedLogSVG(series []annualPoint) string {
	const W, H = 820.0, 195.0
	const L, R, T, B = 42.0, 74.0, 14.0, 22.0
	pw, ph := W-L-R, H-T-B

	baseProf := *series[0].Profit
	cum := 100.0
	type idxPt struct {
		year    int
		profIdx float64
		retIdx  float64
	}
	pts := make([]idxPt, 0, len(series))
	minVal, maxVal := 100.0, 100.0
	for i, p := range series {
		if i > 0 && p.TotalReturn != nil {
			cum *= 1.0 + (*p.TotalReturn)/100.0
		}
		pIdx := 100.0
		if p.Profit != nil && *p.Profit > 0 {
			pIdx = (*p.Profit / baseProf) * 100.0
		}
		if pIdx < minVal {
			minVal = pIdx
		}
		if cum < minVal {
			minVal = cum
		}
		if pIdx > maxVal {
			maxVal = pIdx
		}
		if cum > maxVal {
			maxVal = cum
		}
		pts = append(pts, idxPt{year: p.Year, profIdx: pIdx, retIdx: cum})
	}

	minLog := math.Log10(math.Min(80, minVal*0.85))
	maxLog := math.Log10(maxVal * 1.25)
	n := len(pts)
	xAt := func(i int) float64 { return L + (float64(i)/float64(n-1))*pw }
	yAt := func(v float64) float64 {
		return T + ph - ((math.Log10(math.Max(1, v))-minLog)/(maxLog-minLog))*ph
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg">`, W, H)

	ticks := []float64{100, 300, 1000, 3000, 10000, 30000, 100000}
	for _, tv := range ticks {
		if tv < math.Pow(10, minLog) || tv > math.Pow(10, maxLog) {
			continue
		}
		y := yAt(tv)
		stroke, sw := "#e5e7eb", "0.75"
		if tv == 100 {
			stroke, sw = "#9ca3af", "1"
		}
		lbl := fmt.Sprintf("%.0f", tv)
		if tv >= 1000 {
			lbl = fmt.Sprintf("%.0fk", tv/1000)
		}
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%s"/>`, L, y, W-R, y, stroke, sw)
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="end" font-size="9" fill="#6b7280">%s</text>`, L-5, y+3, lbl)
	}

	var profPoly, retPoly []string
	for i, pt := range pts {
		profPoly = append(profPoly, fmt.Sprintf("%.1f,%.1f", xAt(i), yAt(pt.profIdx)))
		retPoly = append(retPoly, fmt.Sprintf("%.1f,%.1f", xAt(i), yAt(pt.retIdx)))
	}
	fmt.Fprintf(&sb, `<polyline points="%s" fill="none" stroke="#334155" stroke-width="1.9" stroke-linejoin="round"/>`, strings.Join(profPoly, " "))
	fmt.Fprintf(&sb, `<polyline points="%s" fill="none" stroke="#0f766e" stroke-width="2.2" stroke-linejoin="round"/>`, strings.Join(retPoly, " "))

	last := pts[n-1]
	fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="3" fill="#0f766e"/>`, W-R, yAt(last.retIdx))
	fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="3" fill="#334155"/>`, W-R, yAt(last.profIdx))
	fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="9" font-weight="700" fill="#0f766e">%.0f (%.0f×)</text>`, W-R+6, yAt(last.retIdx)+3, last.retIdx, last.retIdx/100)
	fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="9" font-weight="700" fill="#334155">%.0f (%.0f×)</text>`, W-R+6, yAt(last.profIdx)+3, last.profIdx, last.profIdx/100)

	for i, pt := range pts {
		if pt.year%5 == 0 || i == 0 || i == n-1 {
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="9" fill="#6b7280">%d</text>`, xAt(i), H-5, pt.year)
		}
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

func renderSmallMultiplesHTML(series []annualPoint) string {
	type panelSpec struct {
		title    string
		unit     string
		decimals int
		extract  func(annualPoint) *float64
	}
	specs := []panelSpec{
		{title: "Return on assets (ROA)", unit: "%", decimals: 2, extract: func(p annualPoint) *float64 { return p.ROA }},
		{title: "Dividend yield", unit: "%", decimals: 2, extract: func(p annualPoint) *float64 { return p.DivYield }},
		{title: "Price / earnings (P/E)", unit: "×", decimals: 1, extract: func(p annualPoint) *float64 { return p.PE }},
		{title: "Price / book value (P/BV)", unit: "×", decimals: 2, extract: func(p annualPoint) *float64 { return p.PBV }},
	}

	var sb strings.Builder
	sb.WriteString(`<div class="chart-grid-2x2">`)
	for _, spec := range specs {
		var vals []float64
		var lastVal *float64
		maxV := 0.0
		for _, p := range series {
			if v := spec.extract(p); v != nil {
				vals = append(vals, *v)
				lastVal = v
				if *v > maxV {
					maxV = *v
				}
			}
		}
		latestText := "—"
		if lastVal != nil {
			latestText = fmt.Sprintf("%.*f%s", spec.decimals, *lastVal, html.EscapeString(spec.unit))
		}
		sb.WriteString(`<div class="chart-panel">`)
		fmt.Fprintf(&sb, `<div class="chart-panel-head"><span class="chart-panel-title">%s</span><span class="chart-panel-val">%s</span></div>`, html.EscapeString(spec.title), latestText)
		sb.WriteString(renderSingleSmallMultipleSVG(series, spec.extract, vals, maxV, spec.unit))
		sb.WriteString(`</div>`)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func renderSingleSmallMultipleSVG(series []annualPoint, extract func(annualPoint) *float64, vals []float64, maxV float64, unit string) string {
	const W, H = 370.0, 114.0
	const L, R, T, B = 28.0, 12.0, 8.0, 18.0
	pw, ph := W-L-R, H-T-B

	top := niceMax(maxV*1.12, 2)
	n := len(series)
	xAt := func(i int) float64 { return L + (float64(i)/float64(n-1))*pw }
	yAt := func(v float64) float64 { return T + ph - (v/top)*ph }

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg">`, W, H)

	for i, p := range series {
		if p.Year == 2008 || p.Year == 2020 {
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="10" height="%.1f" fill="#f1f5f9"/>`, xAt(i)-5, T, ph)
		}
	}

	for k := 0; k <= 2; k++ {
		v := top * float64(k) / 2.0
		y := yAt(v)
		stroke, sw := "#e5e7eb", "0.7"
		if k == 0 {
			stroke, sw = "#9ca3af", "0.9"
		}
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%s"/>`, L, y, W-R, y, stroke, sw)
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="end" font-size="8.5" fill="#6b7280">%.0f%s</text>`, L-4, y+3, v, html.EscapeString(unit))
	}

	if len(vals) > 0 {
		sorted := slices.Clone(vals)
		slices.Sort(sorted)
		med := sorted[len(sorted)/2]
		ym := yAt(med)
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#94a3b8" stroke-width="0.9" stroke-dasharray="3,3"/>`, L, ym, W-R, ym)
	}

	var linePts []string
	var firstX, lastX, lastY float64
	for i, p := range series {
		if v := extract(p); v != nil {
			x, y := xAt(i), yAt(*v)
			if len(linePts) == 0 {
				firstX = x
			}
			lastX, lastY = x, y
			linePts = append(linePts, fmt.Sprintf("%.1f,%.1f", x, y))
		}
	}
	if len(linePts) > 1 {
		areaPts := fmt.Sprintf("%.1f,%.1f %s %.1f,%.1f", firstX, yAt(0), strings.Join(linePts, " "), lastX, yAt(0))
		fmt.Fprintf(&sb, `<polygon points="%s" fill="rgba(15,118,110,0.08)"/>`, areaPts)
		fmt.Fprintf(&sb, `<polyline points="%s" fill="none" stroke="#0f766e" stroke-width="1.8" stroke-linejoin="round"/>`, strings.Join(linePts, " "))
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="2.8" fill="#0f766e"/>`, lastX, lastY)
	}

	for i, p := range series {
		if p.Year%5 == 0 || i == 0 || i == n-1 {
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="8.5" fill="#6b7280">%d</text>`, xAt(i), H-4, p.Year)
		}
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

func renderDivergingReturnSVG(series []annualPoint) string {
	var pts []annualPoint
	minV, maxV := 0.0, 0.0
	for _, p := range series {
		if p.TotalReturn != nil {
			pts = append(pts, p)
			if *p.TotalReturn < minV {
				minV = *p.TotalReturn
			}
			if *p.TotalReturn > maxV {
				maxV = *p.TotalReturn
			}
		}
	}
	if len(pts) == 0 {
		return ""
	}

	const W, H = 820.0, 185.0
	const L, R, T, B = 38.0, 14.0, 16.0, 22.0
	pw, ph := W-L-R, H-T-B

	lo := -niceMax(math.Abs(minV)*1.25, 1)
	if lo > -20 {
		lo = -20
	}
	hi := niceMax(maxV*1.12, 3)

	n := len(pts)
	xAt := func(i int) float64 { return L + (float64(i)+0.5)*(pw/float64(n)) }
	bw := (pw / float64(n)) * 0.62
	yAt := func(v float64) float64 { return T + ph - ((v-lo)/(hi-lo))*ph }
	y0 := yAt(0)

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg">`, W, H)

	step := niceMax((hi-lo)/4.0, 1)
	for v := math.Ceil(lo/step) * step; v <= hi+0.01; v += step {
		y := yAt(v)
		stroke, sw := "#e5e7eb", "0.75"
		if math.Abs(v) < 0.01 {
			v = 0
			stroke, sw = "#9ca3af", "1"
		}
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%s"/>`, L, y, W-R, y, stroke, sw)
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="end" font-size="9" fill="#6b7280">%.0f%%</text>`, L-5, y+3, v)
	}
	if math.Abs(lo) > 0.01 {
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#9ca3af" stroke-width="1"/>`, L, y0, W-R, y0)
	}

	for i, p := range pts {
		val := *p.TotalReturn
		yv := yAt(val)
		top := math.Min(y0, yv)
		h := math.Max(1.5, math.Abs(yv-y0))
		fill := "#0f766e"
		if val < 0 {
			fill = "#be123c"
		}
		fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="1.5" fill="%s" opacity="0.86"/>`, xAt(i)-bw/2, top, bw, h, fill)
		if val < 0 || val > hi*0.75 || i == n-1 {
			lblY := top - 4
			if val < 0 {
				lblY = top + h + 9.5
			}
			sign := ""
			if val > 0 {
				sign = "+"
			}
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="8.5" font-weight="700" fill="%s">%s%.1f%%</text>`, xAt(i), lblY, fill, sign, val)
		}
		if p.Year%5 == 0 || i == 0 || i == n-1 {
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="9" fill="#6b7280">%d</text>`, xAt(i), H-5, p.Year)
		}
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}
