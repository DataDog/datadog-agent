// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import "go.uber.org/atomic"

// ProcessNotify implements an exchange mechanism for other checks to receive updates on changes
// from process collection
var ProcessNotify = newProcessNotify()

// processNotify implements an exchange mechanism for other checks to receive updates on changes
// from process collection, in the future it can be replaced with a gRPC streaming interface
// to allow notifying checks running in other processes
type processNotify struct {
	// Create times by PID used in the network check
	createTimes *atomic.Value
}

func newProcessNotify() *processNotify {
	return &processNotify{
		createTimes: &atomic.Value{},
	}
}

// UpdateCreateTimes updates create times for collected processes
func (p *processNotify) UpdateCreateTimes(createTimes map[int32]int64) {
	p.createTimes.Store(createTimes)
}

// GetCreateTimes retrieves create times for given PIDs
func (p *processNotify) GetCreateTimes(pids []int32) map[int32]int64 {
	createTimeForPID := make(map[int32]int64)
	if result := p.createTimes.Load(); result != nil {
		createTimesAllPIDs := result.(map[int32]int64)
		for _, pid := range pids {
			if ctime, ok := createTimesAllPIDs[pid]; ok {
				createTimeForPID[pid] = ctime
			}
		}
	}
	return createTimeForPID
}
