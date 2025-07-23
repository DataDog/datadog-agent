// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// processesByPID returns the processes by pid from the process probe for non-linux platforms
// because the workload meta process collection is only available on linux
func (p *ProcessCheck) processesByPID(collectStats bool) (map[int32]*procutil.Process, error) {
	procs, err := p.probe.ProcessesByPID(p.clock.Now(), collectStats)
	if err != nil {
		return nil, err
	}
	return procs, nil
}
