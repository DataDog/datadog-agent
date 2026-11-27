// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Test helpers shared by the eBPF tests (bpf build, run by KMT) and the user-space tests (!bpf build).

package gpu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func getTestSystemContext(tb testing.TB, extraOpts ...systemContextOption) *systemContext {
	opts := []systemContextOption{
		withProcRoot(kernel.ProcFSRoot()),
		withWorkloadMeta(testutil.GetWorkloadMetaMock(tb)),
		withTelemetry(testutil.GetTelemetryMock(tb)),
	}

	opts = append(opts, extraOpts...) // Allow overriding the default options

	sysCtx, err := getSystemContext(opts...)
	require.NoError(tb, err)
	require.NotNil(tb, sysCtx)
	return sysCtx
}

func getMetricsEntry(key model.ProcessStatsKey, stats *model.GPUStats) *model.UtilizationMetrics {
	for _, entry := range stats.ProcessMetrics {
		if entry.Key == key {
			return &entry.UtilizationMetrics
		}
	}

	return nil
}

func getStatsGeneratorForTest(t *testing.T, mockOpts ...testutil.NvmlMockOption) (*statsGenerator, *streamCollection, int64) {
	nvmltestutil.SetupMockNVML(t, mockOpts...)
	sysCtx := getTestSystemContext(t)

	ktime, err := ddebpf.NowNanoseconds()
	require.NoError(t, err)

	streamHandlers := newStreamCollection(sysCtx, testutil.GetTelemetryMock(t), config.New())
	statsGen := newStatsGenerator(sysCtx, streamHandlers, testutil.GetTelemetryMock(t))
	statsGen.lastGenerationKTime = ktime
	statsGen.currGenerationKTime = ktime
	require.NotNil(t, statsGen)

	return statsGen, streamHandlers, ktime
}
