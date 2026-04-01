package dataset

import (
	"strconv"
	"strings"
	"unicode"
)

// TitleMatchScore is in [0,1]: how similar the foreground window title is to the session game name.
// Empty gameName yields -1 (caller should not use as a feature without imputation).
// Uses substring match → 1, else normalized Levenshtein similarity on Unicode runes.
func TitleMatchScore(gameName, windowTitle string) float64 {
	g := normTitle(gameName)
	t := normTitle(windowTitle)
	if g == "" {
		return -1
	}
	if t == "" {
		return 0
	}
	if strings.Contains(t, g) {
		return 1
	}
	gr, tr := []rune(g), []rune(t)
	d := levenshtein(gr, tr)
	maxLen := len(gr)
	if len(tr) > maxLen {
		maxLen = len(tr)
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(d)/float64(maxLen)
}

// TitleMatchScoreCSV formats TitleMatchScore for CSV (empty when game_name is unset).
func TitleMatchScoreCSV(gameName, windowTitle string) string {
	if strings.TrimSpace(gameName) == "" {
		return ""
	}
	s := TitleMatchScore(gameName, windowTitle)
	if s < 0 {
		return ""
	}
	return strconv.FormatFloat(s, 'f', 4, 64)
}

func normTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastSpace && b.Len() > 0 {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func levenshtein(a, b []rune) int {
	m, n := len(a), len(b)
	d := make([][]int, m+1)
	for i := range d {
		d[i] = make([]int, n+1)
		d[i][0] = i
	}
	for j := 0; j <= n; j++ {
		d[0][j] = j
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			d[i][j] = min3(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return d[m][n]
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}
