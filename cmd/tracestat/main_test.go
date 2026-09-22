package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"inbetween/internal/stats"
)

func TestTracestat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.csv")
	var sb strings.Builder
	sb.WriteString(stats.TraceHeader + "\n")
	for i := 0; i < 120; i++ {
		kind := "gen"
		if i%2 == 1 {
			kind = "real"
		}
		ts := float64(i) * 8.333
		if i == 60 {
			ts += 10 // рывок
		}
		fmt.Fprintf(&sb, "%.3f,%s,0.5,%d,%.3f,0.100,16.667,0.300\n", ts, kind, i/2, ts)
	}
	os.WriteFile(path, []byte(sb.String()), 0o644)
	var out bytes.Buffer
	if err := run(path, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "real 60, gen 60") || !strings.Contains(out.String(), "рывков") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
