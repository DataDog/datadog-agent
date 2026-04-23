// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package logssourceimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/goleak"

	compConfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// --- filter function tests ---

func TestIsPauseContainer(t *testing.T) {
	sp, _ := newTestSourceProvider() // pauseFilter=nil → falls back to heuristic
	cases := []struct {
		image string
		want  bool
	}{
		{"pause", true},
		{"k8s.gcr.io/pause", true},
		{"registry.k8s.io/pause:3.9", true},
		{"PAUSE", true},
		{"nginx", false},
		{"my-pause-proxy", true}, // contains "pause" — acceptable false positive for fallback heuristic
		{"", false},
	}
	for _, tc := range cases {
		c := &workloadmeta.Container{
			Image: workloadmeta.ContainerImage{ShortName: tc.image},
		}
		assert.Equal(t, tc.want, sp.isPauseContainer(c), "image=%q", tc.image)
	}
}

func TestIsAgentContainer(t *testing.T) {
	cases := []struct {
		image string
		want  bool
	}{
		{"datadog-agent", true},
		{"gcr.io/datadoghq/datadog-agent", true},
		{"dd-agent", true},
		{"DD-AGENT", true},
		{"nginx", false},
		{"my-datadog-agent-sidecar", true},
	}
	for _, tc := range cases {
		c := &workloadmeta.Container{
			Image: workloadmeta.ContainerImage{ShortName: tc.image},
		}
		assert.Equal(t, tc.want, isAgentContainer(c), "image=%q", tc.image)
	}
}

// --- sourceProvider handle method tests ---

func newTestSourceProvider() (*sourceProvider, *sources.LogSources) {
	ls := sources.NewLogSources()
	sp := newSourceProvider(nil, ls, nil) // wmeta/pauseFilter not needed for direct handle tests
	return sp, ls
}

func runningContainer(id, image string) *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: id},
		Image:    workloadmeta.ContainerImage{ShortName: image},
		Runtime:  workloadmeta.ContainerRuntimeContainerd,
		State:    workloadmeta.ContainerState{Running: true},
	}
}

func TestHandleSet_AddsRunningContainer(t *testing.T) {
	sp, ls := newTestSourceProvider()
	sp.handleSet(runningContainer("abc", "nginx"))
	assert.Len(t, ls.GetSources(), 1)
}

func TestHandleSet_SkipsNonRunning(t *testing.T) {
	sp, ls := newTestSourceProvider()
	c := runningContainer("abc", "nginx")
	c.State.Running = false
	sp.handleSet(c)
	assert.Empty(t, ls.GetSources())
}

func TestHandleSet_SkipsPauseImage(t *testing.T) {
	sp, ls := newTestSourceProvider()
	sp.handleSet(runningContainer("abc", "pause"))
	assert.Empty(t, ls.GetSources())
}

func TestHandleSet_SkipsAgentImage(t *testing.T) {
	sp, ls := newTestSourceProvider()
	sp.handleSet(runningContainer("abc", "datadog-agent"))
	assert.Empty(t, ls.GetSources())
}

func TestHandleSet_Idempotent(t *testing.T) {
	sp, ls := newTestSourceProvider()
	c := runningContainer("abc", "nginx")
	sp.handleSet(c)
	sp.handleSet(c) // second Set for same container ID
	assert.Len(t, ls.GetSources(), 1, "duplicate Set must not add a second LogSource")
}

func TestHandleUnset_RemovesSource(t *testing.T) {
	sp, ls := newTestSourceProvider()
	c := runningContainer("abc", "nginx")
	sp.handleSet(c)
	require.Len(t, ls.GetSources(), 1)
	sp.handleUnset(c)
	assert.Empty(t, ls.GetSources())
}

func TestHandleUnset_UnknownContainerIsNoop(t *testing.T) {
	sp, ls := newTestSourceProvider()
	sp.handleUnset(runningContainer("unknown", "nginx")) // should not panic
	assert.Empty(t, ls.GetSources())
}

func TestHandleUnset_AllowsReAdd(t *testing.T) {
	sp, ls := newTestSourceProvider()
	c := runningContainer("abc", "nginx")
	sp.handleSet(c)
	sp.handleUnset(c)
	sp.handleSet(c) // re-add after removal must work
	assert.Len(t, ls.GetSources(), 1)
}

// --- goroutine lifecycle test ---

func newWMetaMock(t *testing.T) workloadmetamock.Mock {
	t.Helper()
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func TestSourceProvider_GoRoutineExitsCleanly(t *testing.T) {
	wmeta := newWMetaMock(t)
	// Snapshot after wmeta starts its own goroutines so only ours are checked.
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	ls := sources.NewLogSources()
	sp := newSourceProvider(wmeta, ls, nil)

	ctx, cancel := context.WithCancel(context.Background())
	sp.run(ctx)
	cancel()
	sp.wait() // must not hang
}
