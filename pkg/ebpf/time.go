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
	return ts.Sec*int64(time.Second) + ts.Nsec*int64(time.Nanosecond), nil
}
