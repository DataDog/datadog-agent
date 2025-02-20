// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscribers

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	taggerMock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestGPUDetection(t *testing.T) {
	tests := []struct {
		name                string
		events              []workloadmeta.CollectorEvent
		expectedGPUDetected bool
	}{
		{
			name:                "no events",
			events:              []workloadmeta.CollectorEvent{},
			expectedGPUDetected: false,
		},
		{
			name: "non GPU event",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							ID:   "123",
							Kind: workloadmeta.KindContainer,
						},
					},
				},
			},
			expectedGPUDetected: false,
		},
		{
			name: "one GPU event",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "multiple GPU events",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "2",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "False detection bool overwritten",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							ID:   "123",
							Kind: workloadmeta.KindContainer,
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "True detection bool not overwritten",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							ID:   "123",
							Kind: workloadmeta.KindContainer,
						},
					},
				},
			},
			expectedGPUDetected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))
			fakeTagger := taggerMock.SetupFakeTagger(t)

			gpuDetector := NewGPUSubscriber(mockWmeta, fakeTagger)
			go gpuDetector.Run(context.Background())
			defer gpuDetector.Stop(context.Background())

			// Notify subscribers of events
			mockWmeta.Notify(tt.events)

			// Eventually, GPU detector should finish processing all events
			assert.Eventually(t, func() bool {
				return assert.Equal(t, tt.expectedGPUDetected, gpuDetector.IsGPUDetected())
			}, 1*time.Second, 100*time.Millisecond)
		})
	}
}

func TestGetGPUTags(t *testing.T) {
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindGPU,
		ID:   "gpu-1",
	}

	tests := []struct {
		name           string
		detectedGPU    bool
		gpus           []workloadmeta.GPU
		expectedTagMap map[int32][]string
	}{
		{
			name:        "No detected gpu",
			detectedGPU: false,
			gpus: []workloadmeta.GPU{{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: entityID.ID,
				},
				Vendor:     "nvidia",
				Device:     "tesla-v100",
				ActivePIDs: []int{1234},
			}},
			expectedTagMap: nil,
		},
		{
			name:           "Detected gpu with empty workloadmeta",
			detectedGPU:    true,
			gpus:           []workloadmeta.GPU{},
			expectedTagMap: map[int32][]string{},
		},
		{
			name:        "Detected process on gpu",
			detectedGPU: true,
			gpus: []workloadmeta.GPU{
				{
					EntityID: entityID,
					EntityMeta: workloadmeta.EntityMeta{
						Name: entityID.ID,
					},
					Vendor:     "nvidia",
					Device:     "tesla-v100",
					ActivePIDs: []int{1234},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindGPU,
						ID:   "gpu-2",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "gpu-2",
					},
					Vendor:     "nvidia",
					Device:     "tesla-v105",
					ActivePIDs: []int{185},
				},
			},
			expectedTagMap: map[int32][]string{
				1234: {
					"gpu_uuid:gpu-1",
					"gpu_device:tesla-v100",
					"gpu_vendor:nvidia",
				},
				185: {
					"gpu_uuid:gpu-2",
					"gpu_device:tesla-v105",
					"gpu_vendor:nvidia",
				},
			},
		},
		{
			name:        "Detected process on multiple gpus",
			detectedGPU: true,
			gpus: []workloadmeta.GPU{
				{
					EntityID: entityID,
					EntityMeta: workloadmeta.EntityMeta{
						Name: entityID.ID,
					},
					Vendor:     "nvidia",
					Device:     "tesla-v100",
					ActivePIDs: []int{1234, 185},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindGPU,
						ID:   "gpu-2",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "gpu-2",
					},
					Vendor:     "nvidia",
					Device:     "tesla-v105",
					ActivePIDs: []int{185},
				},
			},
			expectedTagMap: map[int32][]string{
				1234: {
					"gpu_uuid:gpu-1",
					"gpu_device:tesla-v100",
					"gpu_vendor:nvidia",
				},
				185: {
					"gpu_uuid:gpu-2",
					"gpu_device:tesla-v105",
					"gpu_device:tesla-v100",
					"gpu_uuid:gpu-1",
					"gpu_vendor:nvidia",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocked dependencies and ProcessCheck
			mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			fakeTagger := taggerMock.SetupFakeTagger(t)
			gpuDetector := NewGPUSubscriber(mockWmeta, fakeTagger)
			gpuDetector.SetGPUDetected(tt.detectedGPU)

			// Populate workloadmeta and tagger stores with mocked data
			for _, gpu := range tt.gpus {
				mockWmeta.Set(&gpu)
				gpuTags := []string{"gpu_uuid:" + gpu.ID, "gpu_device:" + gpu.Device, "gpu_vendor:" + gpu.Vendor}
				fakeTagger.SetTags(types.NewEntityID(types.GPU, gpu.ID), "fake", gpuTags, nil, nil, nil)
			}

			actualTagMap := gpuDetector.GetGPUTags()
			assert.Equal(t, len(tt.expectedTagMap), len(actualTagMap))
			for pid, tagMap := range tt.expectedTagMap {
				assert.ElementsMatch(t, actualTagMap[pid], tagMap)
			}
		})
	}
}
