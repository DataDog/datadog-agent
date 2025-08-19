// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procutil

import (
	"time"
)

// Probe fetches process related info on current host
type Probe interface {
	Close()
	StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error)
	ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error)
	StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error)
}

// Option is config options callback for system-probe
type Option func(p Probe)
