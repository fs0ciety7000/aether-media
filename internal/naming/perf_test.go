package naming

import (
	"strings"
	"testing"
	"time"
)

// TestPathologicalInputs guards against super-linear behaviour on long,
// repetitive names (fuzzers love them).
func TestPathologicalInputs(t *testing.T) {
	inputs := []string{
		strings.Repeat("(a ", 5000),
		strings.Repeat("a.", 10000),
		strings.Repeat("[x] ", 5000),
		strings.Repeat("S01E01-", 3000),
		strings.Repeat("(2010) ", 3000),
		strings.Repeat("1x01x", 3000),
		strings.Repeat("e1-", 5000),
		strings.Repeat("- 1 ", 5000),
		strings.Repeat("Extended ", 5000),
		strings.Repeat("2010 ", 5000),
		strings.Repeat("a/", 5000) + "b.mkv",
		strings.Repeat("Season 1/", 2000) + "S01E01.mkv",
	}
	for i, in := range inputs {
		start := time.Now()
		ParseMovie(in)
		ParseEpisode(in)
		ParseTrack(in)
		CleanTitle(in)
		ParseSeasonFolder(in)
		ParseSeriesFolder(in)
		SortTitle(in)
		ClassifyExtra(in)
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Errorf("input %d took %v", i, d)
		}
	}
}
