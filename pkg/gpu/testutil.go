// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml && test

// this file contains utilities only used for testing within the gpu package. Utilities meant to
// be used by other packages should be placed in the pkg/gpu/testutil package.

package gpu

import (
	"fmt"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// ensureInitPoolsNoTelemetry ensures that the pools are initialized without telemetry, useful for testing
func ensureInitPoolsNoTelemetry() {
	fmt.Println("ensureInitPoolsNoTelemetry")
	initPoolsOnce.Do(func() {
		enrichedKernelLaunchPool = ddsync.NewDefaultTypedPool[enrichedKernelLaunch]()
		kernelSpanPool = ddsync.NewDefaultTypedPool[kernelSpan]()
		memorySpanPool = ddsync.NewDefaultTypedPool[memorySpan]()
	})
}

func withTelemetryEnabledPools(t *testing.T, tm telemetry.Component) {
	// reset the sync.Once for the pools
	initPoolsOnce = sync.Once{}

	// so that now we can call ensureInitPools with the telemetry component
	ensureInitPools(tm)

	// after the current test is finished, reset the sync.Once and restore to non-telemetry enabled pools
	t.Cleanup(func() {
		initPoolsOnce = sync.Once{}
		ensureInitPoolsNoTelemetry()
	})
}
