package stat

import (
	"os"
	"strings"

	"github.com/prometheus/procfs"
)

// CPU is cgroup v2 cpu.stat. The counters are cumulative.
type CPU struct {
	UsageUsec     uint64
	Periods       uint64 // quota periods that had runnable work
	Throttled     uint64 // periods that ran out of quota
	ThrottledUsec uint64
}

const tcpTimeWait = 6

// TimeWait returns -1 where /proc/net isn't available, e.g. outside Linux.
func TimeWait() int {
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return -1
	}
	v4, err := fs.NetTCP()
	if err != nil {
		return -1
	}
	v6, _ := fs.NetTCP6() // missing when IPv6 is off
	n := 0
	for _, s := range append(v4, v6...) {
		if s.St == tcpTimeWait {
			n++
		}
	}
	return n
}

// Sysctls reads each name from /proc/sys, in this process's network namespace for net.*.
func Sysctls(names ...string) map[string]string {
	out := map[string]string{}
	for _, n := range names {
		b, err := os.ReadFile("/proc/sys/" + strings.ReplaceAll(n, ".", "/"))
		if err == nil {
			out[n] = strings.Join(strings.Fields(string(b)), " ")
		}
	}
	return out
}
