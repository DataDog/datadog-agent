// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package system provides various helper functions and types to interact with system information
package system

import (
	"context"
	"sync"

	"go.uber.org/atomic"
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
	panic("not called")
}
