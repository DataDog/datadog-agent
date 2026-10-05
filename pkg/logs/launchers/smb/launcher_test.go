// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package smb

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client/fake"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

// started is a Launcher running against a fake share, fed by real LogSources.
type started struct {
	share    *fake.Share
	clock    *clock.Mock
	launcher *Launcher
	sources  *sources.LogSources
	tracker  *tailers.TailerTracker
	registry *auditorMock.Registry
	out      *collector
}

func startLauncher(t *testing.T, configure ...func(*Launcher)) *started {
	t.Helper()
	configmock.New(t)
	share := fake.New()
	share.Mkdir("app")
	clk := clock.NewMock()
	l := newTestLauncher(share, clk)
	for _, fn := range configure {
		fn(l)
	}
	provider := mock.NewMockProvider()
	st := &started{
		share:    share,
		clock:    clk,
		launcher: l,
		sources:  sources.NewLogSources(),
		tracker:  tailers.NewTailerTracker(),
		registry: auditorMock.NewMockRegistry(),
		out:      newCollector(t, provider.NextPipelineChan()),
	}
	l.Start(st.sources, provider, st.registry, st.tracker)
	t.Cleanup(l.Stop)
	return st
}

func (st *started) waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	require.Eventually(t, cond, testTimeout, time.Millisecond, msg)
}

func TestLauncherTailsAddedSourcesAndStopsRemovedOnes(t *testing.T) {
	st := startLauncher(t)
	st.share.Write("app/app.log", []byte(lines(1, 2)))
	source := newSMBSource("smb-test")

	st.sources.AddSource(source)
	assert.Equal(t, want(1, 2), st.out.waitLines(t, 2))
	st.waitFor(t, func() bool { return source.Status().IsSuccess() }, "source status")
	all := st.tracker.All()
	require.Len(t, all, 1)
	assert.Equal(t, "smb", all[0].GetType())
	assert.Equal(t, identifier("app/app.log"), all[0].GetID())

	// The next scan runs one poll_interval later.
	st.share.Append("app/app.log", []byte(lines(3, 3)))
	st.clock.Add(time.Second)
	assert.Equal(t, want(1, 3), st.out.waitLines(t, 3))

	st.sources.RemoveSource(source)
	st.waitFor(t, func() bool { return st.share.LiveSessions() == 0 }, "the client is closed with its last source")
	assert.Empty(t, st.tracker.All())
	assert.Empty(t, source.GetInputs())
	assert.False(t, st.registry.TailedSources[identifier("app/app.log")])
}

func TestLauncherSharesOneClientPerShareAndAccount(t *testing.T) {
	st := startLauncher(t)
	st.share.Write("app/a.log", []byte("a\n"))
	st.share.Write("app/b.log", []byte("b\n"))
	first := newSMBSource("first", func(c *config.LogsConfig) { c.Path = "app/a.log" })
	second := newSMBSource("second", func(c *config.LogsConfig) { c.Path = "app/b.log" })

	st.sources.AddSource(first)
	st.sources.AddSource(second)
	assert.ElementsMatch(t, []string{"a", "b"}, st.out.waitLines(t, 2))
	assert.Equal(t, 1, st.share.Calls(fake.OpDial))
	assert.Equal(t, 1, st.share.LiveSessions())

	st.sources.RemoveSource(first)
	st.waitFor(t, func() bool { return len(st.tracker.All()) == 1 }, "the first source's tailer stops")
	assert.Equal(t, 1, st.share.LiveSessions(), "the second source still uses the session")
	st.sources.RemoveSource(second)
	st.waitFor(t, func() bool { return st.share.LiveSessions() == 0 }, "the session is closed with its last source")
}

func TestLauncherNeverTailsAFileForTwoSources(t *testing.T) {
	st := startLauncher(t)
	st.share.Write("app/app.log", []byte(lines(1, 2)))
	first := newSMBSource("first")
	second := newSMBSource("second")

	st.sources.AddSource(first)
	st.out.waitLines(t, 2)
	st.sources.AddSource(second)
	st.waitFor(t, func() bool { return second.Status().IsSuccess() }, "second source scanned")
	assert.Equal(t, []string{identifier("app/app.log")}, first.GetInputs())
	assert.Empty(t, second.GetInputs())
	st.share.Append("app/app.log", []byte(lines(3, 3)))
	st.clock.Add(time.Second)
	st.out.waitLines(t, 3)

	st.launcher.Stop()
	st.out.flush()
	assert.Equal(t, want(1, 3), st.out.lines(), "each line is sent once")
	assert.Empty(t, first.GetInputs())
}

func TestLauncherRefusesSourcesInFIPSBuilds(t *testing.T) {
	st := startLauncher(t, func(l *Launcher) { l.builtForFIPS = func() bool { return true } })
	st.share.Write("app/app.log", []byte(lines(1, 1)))
	source := newSMBSource("smb-test")

	st.sources.AddSource(source)
	st.waitFor(t, func() bool { return source.Status().IsError() }, "FIPS status")
	assert.Contains(t, source.Status().GetError(), "not supported in FIPS builds")
	assert.Zero(t, st.share.Calls(fake.OpDial))

	st.sources.RemoveSource(source)
	st.launcher.Stop()
	assert.Empty(t, st.launcher.refused)
}

func TestLauncherValidatesReplayedSources(t *testing.T) {
	configmock.New(t)
	share := fake.New()
	l := newTestLauncher(share, clock.NewMock())
	logSources := sources.NewLogSources()
	// AddSource keeps invalid sources, and a new subscription replays them
	// without validating them.
	invalid := sources.NewLogSource("invalid", &config.LogsConfig{Type: config.SMBType, Path: "app/*.log"})
	logSources.AddSource(invalid)

	provider := mock.NewMockProvider()
	l.Start(logSources, provider, auditorMock.NewMockRegistry(), tailers.NewTailerTracker())
	t.Cleanup(l.Stop)
	require.Eventually(t, func() bool { return invalid.Status().IsError() }, testTimeout, time.Millisecond)
	assert.Contains(t, invalid.Status().GetError(), "smb block")
	assert.Zero(t, share.Calls(fake.OpDial))
}

func TestLauncherIgnoresARemovalDeliveredBeforeItsAddition(t *testing.T) {
	configmock.New(t)
	share := fake.New()
	l := newTestLauncher(share, clock.NewMock())
	l.pipelineProvider = mock.NewMockProvider()
	l.registry = auditorMock.NewMockRegistry()
	source := newSMBSource("smb-test")

	l.removeSource(source)
	l.addSource(context.Background(), source)
	assert.Empty(t, l.scanners)
	assert.Empty(t, l.removedEarly)
}

func TestLauncherStopWithoutStart(_ *testing.T) {
	l := NewLauncher(closeTimeout)
	l.Stop()
	l.Stop()
}

func TestLauncherStopDoesNotWaitForTheServer(t *testing.T) {
	st := startLauncher(t)
	st.share.SetLatency(time.Hour) // a server that never answers
	source := newSMBSource("smb-test")
	st.sources.AddSource(source)
	st.waitFor(t, func() bool { return st.share.Calls(fake.OpDial) == 1 }, "dialing")

	stopped := make(chan struct{})
	go func() {
		st.launcher.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(testTimeout):
		require.FailNow(t, "Stop waited for the server")
	}
}
