package dataset

import (
	"math"
	"testing"
)

func TestTitleMatchScore(t *testing.T) {
	if s := TitleMatchScore("", "Minecraft"); s != -1 {
		t.Fatalf("empty game: got %v", s)
	}
	if s := TitleMatchScore("minecraft", ""); s != 0 {
		t.Fatalf("empty title: got %v", s)
	}
	if s := TitleMatchScore("minecraft", "Minecraft 1.12.2"); math.Abs(s-1) > 1e-6 {
		t.Fatalf("substring: got %v", s)
	}
	if s := TitleMatchScore("minecraft", "Some other"); s <= 0 || s >= 1 {
		t.Fatalf("partial: got %v", s)
	}
}
