// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !aix
// +build !windows,!aix

package watchdog

import (
	"errors"

	"github.com/DataDog/gopsutil/cpu"
)

func cpuTimeUser(pid int32) (float64, error) {
	times, err := cpu.Times(false)
	if err != nil {
		return 0, err
	}
	if len(times) == 0 {
		return 0, errors.New("no CPU times returned. Will report 0 CPU usage.")
	}
	return times[0].User, nil
}
