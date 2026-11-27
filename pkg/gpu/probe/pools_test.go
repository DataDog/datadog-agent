// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && !bpf

package gpu

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

func (m *memoryPools) reset() {
	m.initOnce = sync.Once{}
}

func withTelemetryEnabledPools(t *testing.T, tm telemetry.Component) {
	// reset the pools and the telemetry for them
	memPools.reset()
	ddsync.ResetGlobalPoolTelemetry()

	// so that now we can call ensureInit with the telemetry component
	memPools.ensureInit(tm)

	// after the current test is finished, reset the sync.Once and restore to non-telemetry enabled pools
	t.Cleanup(func() {
		memPools.reset()
		ddsync.ResetGlobalPoolTelemetry()
		memPools.ensureInitNoTelemetry()
	})
}
