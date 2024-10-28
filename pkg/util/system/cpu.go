// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package system provides various helper functions and types to interact with system information
package system

import (
	"context"
	"runtime"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxHostCPUFailedAttempts = 3
)

var (
	hostCPUCount           = atomic.NewInt64(0)
	hostCPUFailedAttempts  int
	hostCPUCountUpdateLock sync.Mutex
	cpuInfoFunc            func(context.Context, bool) (int, error)
)

// HostCPUCount returns the number of logical CPUs from host
func HostCPUCount() int {
	if v := hostCPUCount.Load(); v != 0 {
		return int(v)
	}

	hostCPUCountUpdateLock.Lock()
	defer hostCPUCountUpdateLock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cpuCount, err := cpuInfoFunc(ctx, true)
	if err != nil {
		hostCPUFailedAttempts++
		log.Debugf("Unable to get host cpu count, err: %v", err)

		// To maximize backward compatibility and still be able to return
		// a value which is accurate in most cases.
		// After max attempts, we give up and cache this value
		if hostCPUFailedAttempts >= maxHostCPUFailedAttempts {
			log.Debugf("Permafail while getting host cpu count, will use runtime.NumCPU(), err: %v", err)
			cpuCount = runtime.NumCPU()
		} else {
			return runtime.NumCPU()
		}
	}
	hostCPUCount.Store(int64(cpuCount))

	return cpuCount
}
