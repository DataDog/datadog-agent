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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
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

func TestGetTrainingTagsPreservesProcessAttribution(t *testing.T) {
	socketPath := sptestutil.SystemProbeSocketPath(t, "gpu-training-info")
	server, err := sptestutil.NewSystemProbeTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/training-info":
			_, _ = w.Write([]byte(`[
				{"pid":10,"device_uuid":"GPU-1","training_run_id":"run-a"},
				{"pid":20,"device_uuid":"GPU-1","training_run_id":"run-b"}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}), socketPath)
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Close)

	cache := nvidia.NewTrainingInfoCache(sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath)))
	require.NoError(t, cache.Refresh())
	check := &Check{trainingInfoCache: cache}

	process := func(pid string) workloadmeta.EntityID {
		return workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: pid}
	}
	metric := func(workloads ...workloadmeta.EntityID) nvidia.Sample {
		return nvidia.NewMetric("test", 1, ddmetrics.GaugeType, nvidia.Medium, nil, workloads)
	}

	tests := []struct {
		name     string
		sample   nvidia.Sample
		expected []string
	}{
		{"device-wide sample", metric(), []string{"training_run_id:run-a", "training_run_id:run-b"}},
		{"process of run A", metric(process("10")), []string{"training_run_id:run-a"}},
		{"process of run B", metric(process("20")), []string{"training_run_id:run-b"}},
		{"all processes", metric(process("10"), process("20")), []string{"training_run_id:run-a", "training_run_id:run-b"}},
		{"process without training info", metric(process("30")), nil},
		{"container workload", metric(workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "abc"}), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, check.getTrainingTags(tt.sample, "GPU-1"))
		})
	}
}

func TestGetTrainingTagsDisabled(t *testing.T) {
	check := &Check{}
	assert.Nil(t, check.getTrainingTags(nvidia.NewMetric("test", 1, ddmetrics.GaugeType, nvidia.Medium, nil, nil), "GPU-1"))
}
