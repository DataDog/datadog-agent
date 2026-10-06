// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sptestutil "github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

func TestConfigureTrainingInfoCacheFeatureGating(t *testing.T) {
	tests := []struct {
		name          string
		gpuMonitoring bool
		runType       string
		expectCache   bool
	}{
		{name: "env identifier", gpuMonitoring: true, runType: "env", expectCache: true},
		{name: "label identifier", gpuMonitoring: true, runType: "label"},
		{name: "system probe disabled", gpuMonitoring: false, runType: "env"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkGeneric := newCheck(taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t))
			check, ok := checkGeneric.(*Check)
			require.True(t, ok)

			WithGPUConfigEnabled(t)
			spCfg := pkgconfigsetup.SystemProbe()
			agentCfg := pkgconfigsetup.Datadog()
			spCfg.SetInTest("gpu_monitoring.enabled", tt.gpuMonitoring)
			spCfg.SetInTest("gpu_monitoring.enable_ebpf_probes", false)
			spCfg.SetInTest("gpu_monitoring.prm_endpoint_enabled", false)
			agentCfg.SetInTest("gpu.jobs.run.key", "_RAY_SUBMISSION_ID")
			agentCfg.SetInTest("gpu.jobs.run.type", tt.runType)
			t.Cleanup(func() {
				spCfg.SetInTest("gpu_monitoring.enabled", false)
				spCfg.SetInTest("gpu_monitoring.enable_ebpf_probes", true)
				spCfg.SetInTest("gpu_monitoring.prm_endpoint_enabled", true)
				agentCfg.SetInTest("gpu.jobs.run.key", "")
				agentCfg.SetInTest("gpu.jobs.run.type", "label")
			})

			check.containerProvider = newMockContainerProvider(t, nil)
			require.NoError(t, check.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))
			t.Cleanup(func() { check.Cancel() })

			if tt.expectCache {
				require.NotNil(t, check.trainingInfoCache)
			} else {
				require.Nil(t, check.trainingInfoCache)
			}
		})
	}
}

func TestGetDeviceTagsIncludesTrainingTags(t *testing.T) {
	socketPath := sptestutil.SystemProbeSocketPath(t, "gpu-training-info")
	server, err := sptestutil.NewSystemProbeTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/training-info":
			_, _ = w.Write([]byte(`[{"pid":10,"device_uuid":"GPU-1","training_run_id":"raysubmit_1"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}), socketPath)
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Close)

	cache := nvidia.NewTrainingInfoCache(sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath)))
	require.NoError(t, cache.Refresh())

	deviceTags := make([]string, 1, 4) // spare capacity to detect aliasing
	deviceTags[0] = "gpu_uuid:gpu-1"
	check := &Check{
		deviceTags:        map[string][]string{"GPU-1": deviceTags, "GPU-2": {"gpu_uuid:gpu-2"}},
		trainingInfoCache: cache,
	}

	assert.Equal(t, []string{"gpu_uuid:gpu-1", "training_run_id:raysubmit_1"}, check.getDeviceTags("GPU-1"))
	assert.Equal(t, []string{"gpu_uuid:gpu-2"}, check.getDeviceTags("GPU-2"))
	assert.Equal(t, []string{"gpu_uuid:gpu-1"}, check.deviceTags["GPU-1"])
}
