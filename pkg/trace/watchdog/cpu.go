// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !aix
// +build !windows,!aix

package watchdog

import "github.com/shirou/gopsutil/v3/process"

func cpuTimeUser(pid int32) (float64, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return 0, err
	}
	times, err := p.Times()
	if err != nil {
		return 0, err
	}
	return times.User, nil
}
