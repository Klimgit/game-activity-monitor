package dataset

import "game-activity-monitor/server/internal/titlematch"

func TitleMatchScore(gameName, windowTitle string) float64 {
	return titlematch.TitleMatchScore(gameName, windowTitle)
}

func TitleMatchScoreCSV(gameName, windowTitle string) string {
	return titlematch.TitleMatchScoreCSV(gameName, windowTitle)
}
