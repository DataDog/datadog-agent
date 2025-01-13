// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package providers

import (
	"testing"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
)

func TestGPUProcessEvents(t *testing.T) {
	// the processEvents function doesn't need any of the deps, so make them nill
	provider, err := NewGPUConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	// Cast from the generic factory method
	gpuProvider, ok := provider.(*GPUConfigProvider)
	require.True(t, ok)

	gpuIDs := []string{"gpu-1234", "gpu-5678"}

	var gpuEntityIDs []workloadmeta.EntityID
	var gpuCreateEvents []workloadmeta.Event
	var gpuDestroyEvents []workloadmeta.Event
	for _, gpuID := range gpuIDs {
		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   gpuID,
		}

		entity := &workloadmeta.GPU{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name: entityID.ID,
			},
			Vendor: "nvidia",
			Device: "tesla-v100",
		}

		gpuEntityIDs = append(gpuEntityIDs, entityID)
		gpuCreateEvents = append(gpuCreateEvents, workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: entity})
		gpuDestroyEvents = append(gpuDestroyEvents, workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: entity})
	}

	createBundle := workloadmeta.EventBundle{Events: gpuCreateEvents}
	destroyBundle1 := workloadmeta.EventBundle{Events: gpuDestroyEvents[0:1]}
	destroyBundle2 := workloadmeta.EventBundle{Events: gpuDestroyEvents[1:2]}

	// Multiple events should only create one config
	changes := gpuProvider.processEvents(createBundle)
	require.Len(t, changes.Schedule, 1)
	require.Len(t, changes.Unschedule, 0)
	require.Equal(t, changes.Schedule[0].Name, gpu.CheckName)

	// More events should not create more configs
	changes = gpuProvider.processEvents(createBundle)
	require.Len(t, changes.Schedule, 0)
	require.Len(t, changes.Unschedule, 0)

	// Destroying one GPU should not unschedule the check
	changes = gpuProvider.processEvents(destroyBundle1)
	require.Len(t, changes.Schedule, 0)
	require.Len(t, changes.Unschedule, 0)

	// Destroying the last GPU should unschedule the check
	changes = gpuProvider.processEvents(destroyBundle2)
	require.Len(t, changes.Schedule, 0)
	require.Len(t, changes.Unschedule, 1)
	require.Equal(t, changes.Unschedule[0].Name, gpu.CheckName)
}
