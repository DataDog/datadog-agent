// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

// workloadMetaMockWithGPUShares returns a workloadmeta mock with the default
// GPUs and a container holding half of each of the given devices.
func workloadMetaMockWithGPUShares(t *testing.T, devices []ddnvml.Device) workloadmetamock.Mock {
	wmeta := testutil.GetWorkloadMetaMockWithDefaultGPUs(t)
	shares := make(map[string]workloadmeta.GPUShare, len(devices))
	for _, device := range devices {
		shares[device.GetDeviceInfo().UUID] = workloadmeta.GPUShare{Fraction: 0.5}
	}
	wmeta.Set(&workloadmeta.Container{
		EntityID:  workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "fractional"},
		GPUShares: shares,
	})
	return wmeta
}

func TestSharingCollector(t *testing.T) {
	device := setupMockDevices(t, testutil.WithDeviceCount(1))[0]
	deviceUUID := device.GetDeviceInfo().UUID
	other := testutil.GPUUUIDs[1]

	container := func(id string, shares map[string]workloadmeta.GPUShare) *workloadmeta.Container {
		return &workloadmeta.Container{
			EntityID:  workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: id},
			GPUShares: shares,
		}
	}
	share := func(fraction float64) map[string]workloadmeta.GPUShare {
		return map[string]workloadmeta.GPUShare{deviceUUID: {Fraction: fraction}}
	}
	multiGPU := container("multi", map[string]workloadmeta.GPUShare{deviceUUID: {Fraction: 0.5}, other: {Fraction: 0.25}})

	tests := []struct {
		name       string
		containers []*workloadmeta.Container
		want       map[string]float64
	}{
		{
			name:       "two pods sharing the GPU in halves",
			containers: []*workloadmeta.Container{container("a", share(0.5)), container("b", share(0.5))},
			want:       map[string]float64{"a": 0.5, "b": 0.5},
		},
		{
			name:       "unequal fractions",
			containers: []*workloadmeta.Container{container("a", share(0.3)), container("b", share(0.7))},
			want:       map[string]float64{"a": 0.3, "b": 0.7},
		},
		{
			name:       "a fraction leaving the rest of the GPU unassigned",
			containers: []*workloadmeta.Container{container("a", share(0.5))},
			want:       map[string]float64{"a": 0.5},
		},
		{
			name:       "a container without a share gets nothing, even alone on the GPU",
			containers: []*workloadmeta.Container{container("whole", nil)},
		},
		{
			name:       "nor do containers sharing the GPU without a known share",
			containers: []*workloadmeta.Container{container("a", nil), container("b", nil)},
		},
		{
			name:       "a share that could not be read is not reported",
			containers: []*workloadmeta.Container{container("a", share(0)), container("b", share(0.5))},
			want:       map[string]float64{"b": 0.5},
		},
		{
			name:       "a multi-GPU container reports its share of this GPU only",
			containers: []*workloadmeta.Container{multiGPU, container("a", share(0.5))},
			want:       map[string]float64{"multi": 0.5, "a": 0.5},
		},
		{
			name:       "a share of another GPU does not apply to this one",
			containers: []*workloadmeta.Container{container("elsewhere", map[string]workloadmeta.GPUShare{other: {Fraction: 0.5}})},
		},
		{
			name: "no containers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wmeta := testutil.GetWorkloadMetaMock(t)
			for _, container := range test.containers {
				wmeta.Set(container)
			}
			collector, err := newSharingCollector(device, &CollectorDependencies{Workloadmeta: wmeta})
			require.NoError(t, err)

			samples, err := collector.Collect()
			require.NoError(t, err)

			got := make(map[string]float64)
			for _, sample := range samples {
				metric, ok := sample.(*Metric)
				require.True(t, ok)
				require.Equal(t, sharingFractionMetricName, metric.Name)
				require.Equal(t, ddmetrics.GaugeType, metric.Type)
				// Each fraction belongs to exactly one container, so that it
				// does not get the tags of the others.
				workloads := metric.AssociatedWorkloads()
				require.Len(t, workloads, 1)
				require.Equal(t, workloadmeta.KindContainer, workloads[0].Kind)
				require.NotContains(t, got, workloads[0].ID)
				got[workloads[0].ID] = metric.Value
			}
			if test.want == nil {
				test.want = map[string]float64{}
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSharingCollectorDisabledWithoutKAIScheduler(t *testing.T) {
	assert.True(t, collectorDisabled(sharing, gpuconfig.Config{}))
	assert.False(t, collectorDisabled(sharing, gpuconfig.Config{KAISchedulerEnabled: true}))
}
