package financials

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var amountToken = regexp.MustCompile(`\(?-?\d[\d.,]*\)?`)

// ParseNumber understands common Indonesian and English financial number
// formats. A dash, blank, or n.m. is missing data, never an implicit zero.
func ParseNumber(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.EqualFold(s, "n.m.") || s == "—" || s == "–" || s == "-" {
		return 0, false
	}
	s = strings.ReplaceAll(s, "Rp", "")
	s = strings.ReplaceAll(s, "rp", "")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, " ", "")
	negative := strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")
	s = strings.Trim(s, "()")
	if strings.HasPrefix(s, "-") {
		negative = !negative
		s = strings.TrimPrefix(s, "-")
	}
	if s == "" || !amountToken.MatchString(s) || amountToken.FindString(s) != s {
		return 0, false
	}
	lastDot, lastComma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")
	switch {
	case lastDot >= 0 && lastComma >= 0:
		if lastComma > lastDot {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case lastComma >= 0:
		if digitsAfter(s, lastComma) == 3 && countChar(s, ',') > 1 {
			s = strings.ReplaceAll(s, ",", "")
		} else if digitsAfter(s, lastComma) == 3 && countChar(s, ',') == 1 {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		}
	case lastDot >= 0:
		if digitsAfter(s, lastDot) == 3 && countChar(s, '.') > 1 {
			s = strings.ReplaceAll(s, ".", "")
		} else if digitsAfter(s, lastDot) == 3 && countChar(s, '.') == 1 {
			s = strings.ReplaceAll(s, ".", "")
		}
	}
	value, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if negative {
		value = -value
	}
	return value, true
}

func NumberTokens(line string) []string {
	return amountToken.FindAllString(line, -1)
}

func digitsAfter(s string, index int) int {
	count := 0
	for i := index + 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		count++
	}
	return count
}

func countChar(s string, needle byte) int {
	count := 0
	for i := range s {
		if s[i] == needle {
			count++
		}
	}
	return count
}
