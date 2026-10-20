package stat

import (
	"os"
	"strings"
	"sync"

	"github.com/containerd/cgroups/v3/cgroup2"
)

var cgroup = sync.OnceValues(loadCgroup)

// "/" is the container's own cgroup, given Docker's default private cgroup namespace.
func loadCgroup() (*cgroup2.Manager, error) {
	return cgroup2.Load("/")
}

// ReadCPU reads only the cpu controller, so sampling it every 10 ms stays cheap.
func ReadCPU() (CPU, error) {
	m, err := cgroup()
	if err != nil {
		return CPU{}, err
	}
	s, err := m.StatFiltered(cgroup2.StatCPU)
	if err != nil {
		return CPU{}, err
	}
	c := s.GetCPU()
	return CPU{
		UsageUsec:     c.GetUsageUsec(),
		Periods:       c.GetNrPeriods(),
		Throttled:     c.GetNrThrottled(),
		ThrottledUsec: c.GetThrottledUsec(),
	}, nil
}

// CPUMax is cpu.max: "100000 100000" is one CPU per 100 ms period, "max 100000" no limit.
func CPUMax() string {
	b, _ := os.ReadFile("/sys/fs/cgroup/cpu.max")
	return strings.TrimSpace(string(b))
}
