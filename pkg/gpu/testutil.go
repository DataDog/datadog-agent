// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml && test

// this file contains utilities only used for testing within the gpu package. Utilities meant to
// be used by other packages should be placed in the pkg/gpu/testutil package.

package gpu

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// ensureInitPoolsNoTelemetry ensures that the pools are initialized without telemetry, useful for testing
func (m *memoryPools) ensureInitPoolsNoTelemetry() {
	m.initOnce.Do(func() {
		m.enrichedKernelLaunchPool = ddsync.NewDefaultTypedPool[enrichedKernelLaunch]()
		m.kernelSpanPool = ddsync.NewDefaultTypedPool[kernelSpan]()
		m.memorySpanPool = ddsync.NewDefaultTypedPool[memorySpan]()
	})
}

func (m *memoryPools) reset() {
	m.initOnce = sync.Once{}
}

func withTelemetryEnabledPools(t *testing.T, tm telemetry.Component) {
	// reset the sync.Once for the pools
	memPools.reset()

	// so that now we can call ensureInitPools with the telemetry component
	memPools.ensureInitPools(tm)

	// after the current test is finished, reset the sync.Once and restore to non-telemetry enabled pools
	t.Cleanup(func() {
		memPools.reset()
		memPools.ensureInitPoolsNoTelemetry()
	})
}
