// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

// this file contains utilities only used for testing within the gpu package. Utilities meant to
// be used by other packages should be placed in the pkg/gpu/testutil package.

package gpu

import (
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// ensureInitNoTelemetry ensures that the pools are initialized without telemetry, useful for testing
func (m *memoryPools) ensureInitNoTelemetry() {
	m.initOnce.Do(func() {
		m.enrichedKernelLaunchPool = ddsync.NewDefaultTypedPool[enrichedKernelLaunch]()
		m.kernelSpanPool = ddsync.NewDefaultTypedPool[kernelSpan]()
		m.memorySpanPool = ddsync.NewDefaultTypedPool[memorySpan]()
	})
}
