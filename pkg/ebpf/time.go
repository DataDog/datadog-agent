// +build linux

package ebpf

import (
	"time"

	"golang.org/x/sys/unix"
)

// NowNanoseconds returns a time that can be compared to bpf_ktime_get_ns()
func NowNanoseconds() (int64, error) {
	var ts unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	if err != nil {
		return 0, err
	}
	// int64 cast is necessary because the size of ts.Sec and ts.Nsec is based on architecture
	return int64(ts.Sec)*int64(time.Second) + int64(ts.Nsec)*int64(time.Nanosecond), nil //nolint:unconvert
}
