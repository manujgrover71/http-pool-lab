//go:build !linux

package stat

import "errors"

func ReadCPU() (CPU, error) {
	return CPU{}, errors.New("cgroup v2 is Linux only")
}

func CPUMax() string {
	return ""
}
