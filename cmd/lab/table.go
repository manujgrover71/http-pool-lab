package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
)

type table struct {
	cols map[string]int
	rows [][]string
}

func readTable(path string) (*table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("%s: empty", path)
	}
	t := &table{
		cols: map[string]int{},
		rows: recs[1:],
	}
	for i, c := range recs[0] {
		t.cols[c] = i
	}
	return t, nil
}

func (t *table) s(i int, col string) string {
	return t.rows[i][t.cols[col]]
}

func (t *table) f(i int, col string) float64 {
	v, _ := strconv.ParseFloat(t.s(i, col), 64)
	return v
}

// delta is last minus first, for the cumulative counters.
func (t *table) delta(col string) float64 {
	return t.f(len(t.rows)-1, col) - t.f(0, col)
}

func (t *table) mean(col string) float64 {
	if len(t.rows) == 0 {
		return 0
	}
	var sum float64
	for i := range t.rows {
		sum += t.f(i, col)
	}
	return sum / float64(len(t.rows))
}

func (t *table) max(col string) float64 {
	m := math.Inf(-1)
	for i := range t.rows {
		m = max(m, t.f(i, col))
	}
	return m
}
