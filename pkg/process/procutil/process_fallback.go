// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package procutil

import (
	"fmt"
	"time"

	"github.com/DataDog/gopsutil/process"
)

// NewProcessProbe returns a Probe object
func NewProcessProbe(options ...Option) Probe {
	p := &probe{}
	for _, option := range options {
		option(p)
	}
	return p
}

// probe is an implementation of the process probe for platforms other than Windows or Linux
type probe struct {
}

func (p *probe) Close() {}

func (p *probe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	procs, err := process.AllProcesses()
	if err != nil {
		return nil, err
	}
	return ConvertAllFilledProcessesToStats(procs), nil
}

func (p *probe) ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error) {
	procs, err := process.AllProcesses()
	if err != nil {
		return nil, err
	}
	return ConvertAllFilledProcesses(procs), nil
}

func (p *probe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("StatsWithPermByPID is not implemented in this environment")
}
