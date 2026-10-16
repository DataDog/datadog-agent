// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestNotAllocatableByGPU(t *testing.T) {
	const (
		gpu0 = "GPU-0"
		gpu1 = "GPU-1"
		mig0 = "MIG-0"
		mig1 = "MIG-1"
	)
	plain := func(uuid string) allocatableGPU { return allocatableGPU{uuid: uuid} }
	migParent := allocatableGPU{uuid: gpu0, migEnabled: true, migUUIDs: []string{mig0, mig1}}

	tests := []struct {
		name string
		gpus []allocatableGPU
		ids  []string
		want map[string]bool
	}{
		{
			name: "GPU allocatable",
			gpus: []allocatableGPU{plain(gpu0), plain(gpu1)},
			ids:  []string{gpu0, gpu1},
			want: map[string]bool{gpu0: false, gpu1: false},
		},
		{
			name: "GPU absent while another one is allocatable",
			gpus: []allocatableGPU{plain(gpu0), plain(gpu1)},
			ids:  []string{gpu0},
			want: map[string]bool{gpu0: false, gpu1: true},
		},
		{
			name: "IDs are matched case-insensitively",
			gpus: []allocatableGPU{plain(gpu0)},
			ids:  []string{"gpu-0"},
			want: map[string]bool{gpu0: false},
		},
		{
			name: "time-slicing replicas",
			gpus: []allocatableGPU{plain(gpu0), plain(gpu1)},
			ids:  []string{gpu0 + "::0", gpu0 + "::1", gpu1 + "::1"},
			want: map[string]bool{gpu0: false, gpu1: false},
		},
		{
			name: "time-sliced GPU without any replica",
			gpus: []allocatableGPU{plain(gpu0), plain(gpu1)},
			ids:  []string{gpu0 + "::0", gpu0 + "::1"},
			want: map[string]bool{gpu0: false, gpu1: true},
		},
		{
			name: "MIG parent with all slices allocatable",
			gpus: []allocatableGPU{migParent, plain(gpu1)},
			ids:  []string{mig0, mig1, gpu1},
			want: map[string]bool{gpu0: false, gpu1: false},
		},
		{
			name: "MIG parent with one of two slices missing",
			gpus: []allocatableGPU{migParent, plain(gpu1)},
			ids:  []string{mig0, gpu1},
			want: map[string]bool{gpu0: true, gpu1: false},
		},
		{
			name: "MIG parent with all slices missing while another GPU is allocatable",
			gpus: []allocatableGPU{migParent, plain(gpu1)},
			ids:  []string{gpu1},
			want: map[string]bool{gpu0: true, gpu1: false},
		},
		{
			name: "time-sliced MIG slices",
			gpus: []allocatableGPU{migParent},
			ids:  []string{mig0 + "::0", mig0 + "::1", mig1 + "::0"},
			want: map[string]bool{gpu0: false},
		},
		{
			name: "MIG enabled but advertised as a whole GPU (MIG strategy none)",
			gpus: []allocatableGPU{migParent},
			ids:  []string{gpu0},
			want: map[string]bool{gpu0: false},
		},
		{
			name: "MIG enabled without slices is not reported",
			gpus: []allocatableGPU{{uuid: gpu0, migEnabled: true}, plain(gpu1)},
			ids:  []string{gpu1},
			want: map[string]bool{gpu0: false, gpu1: false},
		},
		{
			name: "no allocatable IDs: not managed",
			gpus: []allocatableGPU{plain(gpu0), plain(gpu1)},
			ids:  nil,
			want: nil,
		},
		{
			name: "only unknown and index IDs: not managed",
			gpus: []allocatableGPU{plain(gpu0), plain(gpu1)},
			ids:  []string{"0", "1", "GPU-unknown", "MIG-unknown::0", "example.com-device"},
			want: nil,
		},
		{
			name: "unknown and index IDs are ignored when managed",
			gpus: []allocatableGPU{plain(gpu0), plain(gpu1)},
			ids:  []string{"0", "1", "GPU-unknown", gpu1},
			want: map[string]bool{gpu0: true, gpu1: false},
		},
		{
			name: "no GPU on the host",
			gpus: nil,
			ids:  []string{gpu0},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, notAllocatableByGPU(tt.gpus, tt.ids))
		})
	}
}

func TestEmitNotAllocatable(t *testing.T) {
	gpu0, gpu1 := testutil.GPUUUIDs[0], testutil.GPUUUIDs[1]
	notAllocatableTags := func(uuid string) []string {
		return []string{"unavailable_reason:not_allocatable", "gpu_uuid:" + uuid}
	}

	tests := []struct {
		name     string
		ids      []string
		err      error
		excluded []string
		// want maps the GPU UUID to the expected value; GPUs not listed must not report the reason
		want map[string]float64
	}{
		{
			name: "one GPU is missing",
			ids:  []string{gpu0 + "::0"},
			want: map[string]float64{gpu0: 0, gpu1: 1},
		},
		{
			name: "all GPUs allocatable",
			ids:  []string{gpu0, gpu1},
			want: map[string]float64{gpu0: 0, gpu1: 0},
		},
		{
			name:     "excluded GPU is not reported",
			ids:      []string{gpu0},
			excluded: []string{gpu1},
			want:     map[string]float64{gpu0: 0},
		},
		{
			name: "no GPU can be tied to an allocatable ID",
			ids:  []string{"0", "1"},
		},
		{
			name: "kubelet error",
			err:  errors.New("unimplemented"),
		},
		{
			name: "not applicable on this host",
			err:  errAllocatableNotApplicable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2))
			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			wmeta := testutil.GetWorkloadMetaMock(t)
			senderManager := mocksender.CreateDefaultDemultiplexer(t)
			mockSender := mocksender.NewMockSender(t, "gpu")
			mockSender.SetupAcceptAll()

			check := newConfiguredGPUCheck(t, fakeTagger, wmeta, senderManager, map[int]string{})
			check.allocatableIDsFn = func(context.Context) ([]string, error) { return tt.ids, tt.err }
			for _, uuid := range tt.excluded {
				check.excludedDeviceUUIDs[strings.ToLower(uuid)] = struct{}{}
			}

			require.NoError(t, check.deviceCache.Refresh())
			now := time.Now()
			require.NoError(t, check.emitMetrics(mockSender, map[string][]*workloadmeta.Container{}, nil, now))
			ts := float64(now.UnixNano()) / float64(time.Second)

			for uuid, value := range tt.want {
				mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", "gpu.device.unavailable", value, "", notAllocatableTags(uuid), ts)
			}
			reported := 0
			for _, call := range mockSender.Mock.Calls {
				if call.Method != "GaugeWithTimestamp" || call.Arguments.String(0) != "gpu.device.unavailable" {
					continue
				}
				for _, tag := range call.Arguments.Get(3).([]string) {
					if tag == "unavailable_reason:not_allocatable" {
						reported++
					}
				}
			}
			require.Equal(t, len(tt.want), reported)
		})
	}
}
