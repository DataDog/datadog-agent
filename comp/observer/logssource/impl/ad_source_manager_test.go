// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// newTestADSetup returns a fresh (sp, mgr, ls) triple for AD dedupe tests.
func newTestADSetup(t *testing.T) (*sourceProvider, *adSourceManager, *sources.LogSources) {
	t.Helper()
	ls := sources.NewLogSources()
	sp := newSourceProvider(nil, ls, nil)
	svc := service.NewServices()
	mgr := newADSourceManager(ls, svc, sp)
	return sp, mgr, ls
}

func containerADSource(id string) *sources.LogSource {
	return sources.NewLogSource("ad-container-"+id, &logsconfig.LogsConfig{
		Type:       logsconfig.ContainerdType,
		Identifier: id,
	})
}

func fileADSource(name, path string) *sources.LogSource {
	return sources.NewLogSource(name, &logsconfig.LogsConfig{
		Type: logsconfig.FileType,
		Path: path,
	})
}

// TestADSourceManager_FileConfig verifies that a file-type AD source (e.g. from
// conf.d) is forwarded to LogSources without any suppression side-effects.
func TestADSourceManager_FileConfig(t *testing.T) {
	_, mgr, ls := newTestADSetup(t)

	src := fileADSource("my-integration", "/var/log/app.log")
	mgr.AddSource(src)

	srcs := ls.GetSources()
	require.Len(t, srcs, 1)
	assert.Equal(t, src, srcs[0])
}

// TestADSourceManager_ContainerSourceSuppressesExistingGeneric verifies that
// adding a container-type AD source removes an already-active generic source
// for the same container ID.
func TestADSourceManager_ContainerSourceSuppressesExistingGeneric(t *testing.T) {
	sp, mgr, ls := newTestADSetup(t)

	// Generic source added first (as if workloadmeta fired a Set event).
	c := runningContainer("abc123", "nginx")
	sp.handleSet(c)
	require.Len(t, ls.GetSources(), 1, "generic source should be present before AD")

	// AD source for the same container arrives.
	adSrc := containerADSource("abc123")
	mgr.AddSource(adSrc)

	srcs := ls.GetSources()
	require.Len(t, srcs, 1, "only the AD source should remain")
	assert.Equal(t, adSrc, srcs[0], "AD source should be the active source")
}

// TestADSourceManager_SuppressedIdentifierPreventsNewGenericSource verifies
// that a workloadmeta Set event for a suppressed container ID does not create
// a duplicate generic source.
func TestADSourceManager_SuppressedIdentifierPreventsNewGenericSource(t *testing.T) {
	sp, mgr, ls := newTestADSetup(t)

	// AD source added first — suppresses the identifier before any generic source.
	adSrc := containerADSource("abc123")
	mgr.AddSource(adSrc)

	// A workloadmeta Set fires for the same container.
	c := runningContainer("abc123", "nginx")
	sp.handleSet(c)

	// Only the AD source should be in LogSources.
	srcs := ls.GetSources()
	require.Len(t, srcs, 1)
	assert.Equal(t, adSrc, srcs[0])
}

// TestADSourceManager_RemoveADSourceReleasesSuppression verifies that removing
// the AD source releases suppression so generic collection can resume.
func TestADSourceManager_RemoveADSourceReleasesSuppression(t *testing.T) {
	sp, mgr, ls := newTestADSetup(t)

	adSrc := containerADSource("abc123")
	mgr.AddSource(adSrc)

	// Simulate a workloadmeta Set while suppressed — must not add a generic source.
	c := runningContainer("abc123", "nginx")
	sp.handleSet(c)
	require.Len(t, ls.GetSources(), 1, "only AD source while suppressed")

	// AD source is removed (e.g. k8s annotation deleted).
	mgr.RemoveSource(adSrc)
	assert.Empty(t, ls.GetSources(), "no sources after AD source removed")

	// A new workloadmeta Set (e.g. container restarted) must now produce a generic source.
	sp.handleSet(c)
	srcs := ls.GetSources()
	require.Len(t, srcs, 1, "generic source should be re-added after suppression released")
	assert.Equal(t, logsconfig.ContainerdType, srcs[0].Config.Type)
	assert.Equal(t, "abc123", srcs[0].Config.Identifier)
}

// TestIsContainerSource covers the helper used by adSourceManager.
func TestIsContainerSource(t *testing.T) {
	cases := []struct {
		name string
		src  *sources.LogSource
		want bool
	}{
		{
			name: "containerd with identifier",
			src:  sources.NewLogSource("x", &logsconfig.LogsConfig{Type: logsconfig.ContainerdType, Identifier: "id1"}),
			want: true,
		},
		{
			name: "docker with identifier",
			src:  sources.NewLogSource("x", &logsconfig.LogsConfig{Type: logsconfig.DockerType, Identifier: "id1"}),
			want: true,
		},
		{
			name: "containerd no identifier",
			src:  sources.NewLogSource("x", &logsconfig.LogsConfig{Type: logsconfig.ContainerdType}),
			want: false,
		},
		{
			name: "file type",
			src:  sources.NewLogSource("x", &logsconfig.LogsConfig{Type: logsconfig.FileType, Identifier: "id1"}),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isContainerSource(tc.src))
		})
	}
}

// TestSourceProvider_SuppressIdentifier_DirectAPI tests the low-level
// suppressIdentifier / unsuppressIdentifier methods in isolation.
func TestSourceProvider_SuppressIdentifier_DirectAPI(t *testing.T) {
	sp, ls := newTestSourceProvider()

	// Add a generic source, then suppress it.
	c := runningContainer("c1", "nginx")
	sp.handleSet(c)
	require.Len(t, ls.GetSources(), 1)

	sp.suppressIdentifier("c1")
	assert.Empty(t, ls.GetSources(), "generic source removed on suppress")

	// While suppressed, handleSet is a no-op.
	sp.handleSet(c)
	assert.Empty(t, ls.GetSources())

	// After unsuppress the generic source can be re-created.
	sp.unsuppressIdentifier("c1")
	sp.handleSet(c)
	assert.Len(t, ls.GetSources(), 1)
}

// TestSourceProvider_SuppressUnknownIdentifier verifies that suppressing an ID
// that has no active generic source is safe (no panic, no spurious removes).
func TestSourceProvider_SuppressUnknownIdentifier(t *testing.T) {
	sp, ls := newTestSourceProvider()
	assert.NotPanics(t, func() { sp.suppressIdentifier("nonexistent") })
	assert.Empty(t, ls.GetSources())
}

// Ensure the workloadmeta runtime values used in handleSet match the
// logsconfig constants relied on by isContainerSource.
func TestContainerRuntimeMatchesLogsConfigType(t *testing.T) {
	assert.Equal(t, logsconfig.DockerType, string(workloadmeta.ContainerRuntimeDocker))
	assert.Equal(t, logsconfig.ContainerdType, string(workloadmeta.ContainerRuntimeContainerd))
}

// TestSuppressAndHandleSet_Race exercises suppressIdentifier and handleSet
// concurrently so that the -race detector can catch data races on the shared
// suppressedIDs / activeSources maps.
func TestSuppressAndHandleSet_Race(t *testing.T) {
	sp, mgr, _ := newTestADSetup(t)
	c := runningContainer("race-id", "nginx")
	adSrc := containerADSource("race-id")

	const iterations = 200
	var wg sync.WaitGroup

	// Goroutine 1: repeatedly add/remove the AD source (suppress/unsuppress path).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			mgr.AddSource(adSrc)
			mgr.RemoveSource(adSrc)
		}
	}()

	// Goroutine 2: repeatedly simulate workloadmeta Set events for the same container.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			sp.handleSet(c)
			sp.handleUnset(c)
		}
	}()

	wg.Wait()
	// No assertion on final state — the goal is for -race to find no data races.
}
