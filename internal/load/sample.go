package load

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/manujgrover71/http-pool-lab/internal/csvw"
	"github.com/manujgrover71/http-pool-lab/internal/stat"
)

// sampleCPU runs inside the cgroup it measures, so a throttled period delays its samples
// too. Each row carries the time it was actually read, not the tick.
func sampleCPU(
	dir string, start time.Time, every time.Duration, stop <-chan struct{},
) error {
	if _, err := stat.ReadCPU(); err != nil {
		return nil // not in a cgroup v2 container
	}
	f, err := csvw.Create(
		filepath.Join(dir, "cpu.csv"), "t_ms", "usage_usec", "nr_periods", "nr_throttled",
		"throttled_usec",
	)
	if err != nil {
		return err
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if s, err := stat.ReadCPU(); err == nil {
				f.Write(
					time.Since(start).Milliseconds(), s.UsageUsec, s.Periods, s.Throttled,
					s.ThrottledUsec,
				)
			}
		case <-stop:
			return f.Close()
		}
	}
}

var serverColumns = []string{
	"usage_usec", "nr_periods", "nr_throttled", "throttled_usec", "opened", "closed", "active",
	"requests", "timewait",
}

// sampleServer has its own client, on the stats port, so its connection never shows up in
// the counts it reads.
func sampleServer(url, dir string, start time.Time, stop <-chan struct{}) error {
	if url == "" {
		return nil
	}
	columns := append([]string{"t_ms"}, serverColumns...)
	f, err := csvw.Create(filepath.Join(dir, "server.csv"), columns...)
	if err != nil {
		return err
	}
	hc := &http.Client{Timeout: 2 * time.Second}
	readServer(hc, url, start, f)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			readServer(hc, url, start, f)
		case <-stop:
			readServer(hc, url, start, f)
			return f.Close()
		}
	}
}

// readServer skips the row when the server doesn't answer; the next one still has the
// cumulative counters.
func readServer(hc *http.Client, url string, start time.Time, f *csvw.File) {
	resp, err := hc.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var m map[string]int64
	if json.NewDecoder(resp.Body).Decode(&m) != nil {
		return
	}
	row := []any{time.Since(start).Milliseconds()}
	for _, k := range serverColumns {
		row = append(row, m[k])
	}
	f.Write(row...)
}
