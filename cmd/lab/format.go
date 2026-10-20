package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
)

func pct(vs []float64, p float64) float64 {
	if len(vs) == 0 {
		return math.NaN()
	}
	s := slices.Clone(vs)
	slices.Sort(s)
	return s[min(len(s)-1, int(float64(len(s))*p/100))]
}

func dur(us float64) string {
	switch {
	case math.IsNaN(us):
		return "-"
	case us >= 1e6:
		return fmt.Sprintf("%.2f s", us/1e6)
	case us >= 1e3:
		return fmt.Sprintf("%.1f ms", us/1e3)
	default:
		return fmt.Sprintf("%.0f µs", us)
	}
}

func num(v float64) string {
	return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
}

func share(n, of float64) string {
	if of == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*n/of)
}

// count treats -1 as unreadable.
func count(v float64) string {
	if v < 0 || math.IsInf(v, 0) {
		return "-"
	}
	return num(v)
}
