//go:build !linux
// +build !linux

package config

import "errors"

func getCgroupCPULimit() (float64, error) {
	return 0, errors.New("cgroup cpu limit not support outside linux environments")
}
