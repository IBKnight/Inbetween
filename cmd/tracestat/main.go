// Command tracestat analyzes a live-mode CSV trace (inbetween -trace file.csv).
//
//	go run ./cmd/tracestat trace.csv
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"inbetween/internal/stats"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: tracestat trace.csv")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

func run(path string, out io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		return err
	}
	if strings.Join(head, ",") != stats.TraceHeader {
		return fmt.Errorf("неожиданный заголовок: %v", head)
	}
	var (
		intervals, late, cpu []float64
		real, gen            int
		prev                 = -1.0
	)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		t, _ := strconv.ParseFloat(rec[0], 64)
		if rec[1] == "real" {
			real++
		} else {
			gen++
		}
		l, _ := strconv.ParseFloat(rec[5], 64)
		c, _ := strconv.ParseFloat(rec[7], 64)
		late = append(late, l)
		cpu = append(cpu, c)
		if prev >= 0 {
			intervals = append(intervals, t-prev)
		}
		prev = t
	}
	if len(intervals) < 2 {
		return fmt.Errorf("слишком мало строк")
	}
	pc := func(v []float64) string {
		return fmt.Sprintf("p50=%.2f p90=%.2f p99=%.2f max=%.2f", stats.Percentile(v, 50), stats.Percentile(v, 90), stats.Percentile(v, 99), stats.Percentile(v, 100))
	}
	med := stats.Percentile(intervals, 50)
	hitches := 0
	for _, v := range intervals {
		if v > 1.5*med {
			hitches++
		}
	}
	dur := prev / 1000
	fmt.Fprintf(out, "показов: %d (real %d, gen %d) за %.1f с, в среднем %.1f fps\n", real+gen, real, gen, dur, float64(real+gen)/dur)
	fmt.Fprintf(out, "интервал между Present, мс: %s\n", pc(intervals))
	fmt.Fprintf(out, "опоздание от расписания, мс: %s\n", pc(late))
	fmt.Fprintf(out, "CPU на показ, мс: %s\n", pc(cpu))
	fmt.Fprintf(out, "рывков (> 1.5× медианы %.2f мс): %d (%.2f%%)\n", med, hitches, 100*float64(hitches)/float64(len(intervals)))

	const maxBin = 40
	var bins [maxBin + 1]int
	for _, v := range intervals {
		b := int(v)
		if b > maxBin {
			b = maxBin
		}
		if b < 0 {
			b = 0
		}
		bins[b]++
	}
	fmt.Fprintln(out, "гистограмма интервалов (мс):")
	for i, n := range bins {
		if n == 0 {
			continue
		}
		bar := strings.Repeat("#", max(1, n*60/len(intervals)))
		label := fmt.Sprintf("%2d-%2d", i, i+1)
		if i == maxBin {
			label = fmt.Sprintf(">=%d ", maxBin)
		}
		fmt.Fprintf(out, "  %s %6d %s\n", label, n, bar)
	}
	return nil
}
