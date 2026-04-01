package dataset

import "game-activity-monitor/server/internal/titlematch"

// TitleMatchScore delegates to titlematch (shared with ML inference features).
func TitleMatchScore(gameName, windowTitle string) float64 {
	return titlematch.TitleMatchScore(gameName, windowTitle)
}

// TitleMatchScoreCSV formats TitleMatchScore for CSV (empty when game_name is unset).
func TitleMatchScoreCSV(gameName, windowTitle string) string {
	return titlematch.TitleMatchScoreCSV(gameName, windowTitle)
}
