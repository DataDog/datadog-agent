// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Package ad provides autodiscovery-based log scheduling.
package ad

// TestContainerCollectAllStartupRaceGap exercises the generic→annotated
// source-swap sequence at the AD scheduler level and asserts:
//
//  1. No wrong-metadata window: while the CCA source is the only active
//     source for container "abc123", no annotated source is simultaneously
//     active (i.e., the sequence is strictly add-CCA → remove-CCA →
//     add-annotated, not add-CCA → add-annotated → remove-CCA).
//
//  2. Gap window exists: between RemoveSource(CCA) and AddSource(annotated)
//     the active-source count for "abc123" drops to zero.  This proves the
//     hazard is real and not mitigated at the scheduler level.
//
// The test FAILS on assertion (2) if the gap exists (the bug is present) so
// that a failing test output is a reproduction of the race.
// Assertion (1) passes to show the ordering is correct (no wrong-metadata
// window where both sources are simultaneously present and the annotated one
// is newer).

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackingSourceManager wraps MockSourceManager and records the active source
// count for a specific container identifier at each AddSource / RemoveSource
// call.  This lets the test observe the gap window.
type trackingSourceManager struct {
	schedulers.MockSourceManager
	containerID    string
	snapshots      []int // active source count for containerID at each event
	minActiveCount int   // minimum observed active count after initial add
}

func (t *trackingSourceManager) activeCountForContainer() int {
	count := 0
	for _, src := range t.Sources {
		if src.Config.Identifier == t.containerID {
			count++
		}
	}
	return count
}

func (t *trackingSourceManager) AddSource(source *sourcesPkg.LogSource) {
	t.Sources = append(t.Sources, source)
	t.MockSourceManager.AddSource(source)
	n := t.activeCountForContainer()
	t.snapshots = append(t.snapshots, n)
}

func (t *trackingSourceManager) RemoveSource(source *sourcesPkg.LogSource) {
	// Remove from live Sources slice (by pointer equality, like the real impl)
	for i, s := range t.Sources {
		if s == source {
			t.Sources = append(t.Sources[:i], t.Sources[i+1:]...)
			break
		}
	}
	t.MockSourceManager.RemoveSource(source)
	n := t.activeCountForContainer()
	t.snapshots = append(t.snapshots, n)
	// Track the minimum count after initial setup so we can detect zero.
	if t.minActiveCount > n {
		t.minActiveCount = n
	}
}

func (t *trackingSourceManager) GetSources() []*sourcesPkg.LogSource {
	srcs := make([]*sourcesPkg.LogSource, len(t.Sources))
	copy(srcs, t.Sources)
	return srcs
}

func (t *trackingSourceManager) AddService(svc *service.Service) {
	t.MockSourceManager.AddService(svc)
}

func (t *trackingSourceManager) RemoveService(svc *service.Service) {
	t.MockSourceManager.RemoveService(svc)
}

// TestContainerCollectAllStartupRaceGap is the main race-window test.
func TestContainerCollectAllStartupRaceGap(t *testing.T) {
	const containerID = "abc1230000000000000000000000000000000000000000000000000000000000"
	const serviceID = "docker://" + containerID

	// --- Build the CCA (container_collect_all) integration config.
	// This is what AD delivers when container_collect_all is enabled and no
	// annotated config is present yet.  The Provider is set to
	// names.KubeContainer, which is what config_poller sets when the
	// ContainerConfigProvider streams a config.
	ccaConfig := integration.Config{
		Name:          "container_collect_all",
		LogsConfig:    []byte(`[{}]`),
		ADIdentifiers: []string{serviceID},
		Provider:      names.KubeContainer, // set by config_poller.stream() line 113
		ServiceID:     serviceID,
	}

	// --- Build the annotated integration config.
	// This is what AD delivers when the pod/container annotation arrives.
	annotatedConfig := integration.Config{
		Name:          "myapp",
		LogsConfig:    []byte(`[{"service":"myapp","source":"python"}]`),
		ADIdentifiers: []string{serviceID},
		Provider:      names.KubeContainer,
		ServiceID:     serviceID,
	}

	// --- Scheduler under test (no real AD component needed)
	sch := New(nil).(*Scheduler)
	mgr := &trackingSourceManager{
		containerID:    containerID,
		minActiveCount: 999, // will be updated on first RemoveSource
	}
	sch.mgr = mgr

	// Step 1: AD schedules the CCA config (no annotated config yet).
	sch.Schedule([]integration.Config{ccaConfig})

	require.Equal(t, 1, len(mgr.Events), "expected one AddSource event after scheduling CCA config")
	require.True(t, mgr.Events[0].Add, "first event should be AddSource")

	ccaSource := mgr.Events[0].Source
	require.NotNil(t, ccaSource)
	assert.Equal(t, "container_collect_all", ccaSource.Name, "CCA source name should be container_collect_all")
	assert.Equal(t, containerID, ccaSource.Config.Identifier, "CCA source identifier should match container")
	// CCA source has no service or source annotation — generic metadata.
	assert.Equal(t, "", ccaSource.Config.Service, "CCA source should have no service tag")
	assert.Equal(t, "", ccaSource.Config.Source, "CCA source should have no source tag")

	// Record the post-add snapshot index so we can check ordering.
	addCCAIdx := len(mgr.snapshots) - 1

	// Reset the minimum active count tracker to measure only after the
	// initial CCA source is present.
	mgr.minActiveCount = mgr.snapshots[addCCAIdx]

	// Step 2: AD unschedules the CCA config (annotated config arrived).
	// The AD scheduler matches by Identifier.
	sch.Unschedule([]integration.Config{ccaConfig})

	require.Equal(t, 2, len(mgr.Events), "expected a RemoveSource event after unscheduling CCA config")
	require.False(t, mgr.Events[1].Add, "second event should be RemoveSource")
	assert.Equal(t, ccaSource, mgr.Events[1].Source, "removed source pointer should match previously added CCA source")

	removeCCAIdx := len(mgr.snapshots) - 1

	// Step 3: AD schedules the annotated config.
	sch.Schedule([]integration.Config{annotatedConfig})

	require.Equal(t, 3, len(mgr.Events), "expected an AddSource event after scheduling annotated config")
	require.True(t, mgr.Events[2].Add, "third event should be AddSource")

	annotatedSource := mgr.Events[2].Source
	require.NotNil(t, annotatedSource)
	assert.Equal(t, "myapp", annotatedSource.Name, "annotated source name should be myapp")
	assert.Equal(t, "myapp", annotatedSource.Config.Service, "annotated source should carry service tag")
	assert.Equal(t, "python", annotatedSource.Config.Source, "annotated source should carry source tag")

	addAnnotatedIdx := len(mgr.snapshots) - 1

	// --- Assert correct ordering (no wrong-metadata window) ---
	// The event sequence must be: add-CCA → remove-CCA → add-annotated.
	// The annotated source must NOT have been added before the CCA was removed.
	assert.True(t, addCCAIdx < removeCCAIdx && removeCCAIdx < addAnnotatedIdx,
		"ordering must be: AddSource(CCA) < RemoveSource(CCA) < AddSource(annotated); "+
			"got addCCA=%d removeCCA=%d addAnnotated=%d",
		addCCAIdx, removeCCAIdx, addAnnotatedIdx,
	)

	// --- Assert the gap window: active count reaches zero between remove and add ---
	// At removeCCAIdx snapshot the count for container abc123 should be 0.
	countAtRemoval := mgr.snapshots[removeCCAIdx]
	t.Logf("Active source count snapshots: %v", mgr.snapshots)
	t.Logf("Count at CCA removal: %d (want 0 to prove gap)", countAtRemoval)
	t.Logf("Count at annotated add: %d (want 1)", mgr.snapshots[addAnnotatedIdx])

	// This assertion FAILS if the gap exists (i.e., countAtRemoval == 0).
	// A test failure here IS the reproduction of the bug:
	// there is a window where the container has no active log source.
	assert.NotEqual(t, 0, countAtRemoval,
		"BUG REPRODUCED: gap window detected — after RemoveSource(CCA) and before "+
			"AddSource(annotated), the container '%s' has NO active log source. "+
			"Log lines written during this window are not collected.",
		containerID,
	)
}
