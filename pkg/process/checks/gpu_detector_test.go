// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"go.uber.org/fx"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
)

// Testcases:
// no detected gpu
// detected gpu but workloadmeta is empty
// detected gpu and process asociated to one gpu
// detected gpu and process not active pid
// detectd gpu and processes associated with multiple gpus

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
			name:        "no detected gpu",
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
			name:           "detected gpu with empty workloadmeta",
			detectedGPU:    true,
			gpus:           []workloadmeta.GPU{},
			expectedTagMap: map[int32][]string{},
		},
		{
			name:        "detected process on gpu",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock workloadmeta
			mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)).(workloadmetamock.Mock)

			// Create gpu detector
			gpuDetector := NewGPUDetector(mockWmeta)

			// Populate workloadmeta store with mocked gpus
			for _, gpu := range tt.gpus {
				mockWmeta.Set(&gpu)
			}

			gpuDetector.detectedGPU.Store(tt.detectedGPU)

			actualTagMap := gpuDetector.GetGPUTags()
			assert.Equal(t, actualTagMap, tt.expectedTagMap)
		})
	}
}
