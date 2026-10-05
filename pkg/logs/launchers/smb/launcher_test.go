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

// returnsWithin fails the test if fn does not return within testTimeout.
func returnsWithin(t *testing.T, msg string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(testTimeout):
		require.FailNow(t, msg)
	}
}

func TestLauncherStopDoesNotWaitForLogoffs(t *testing.T) {
	st := startLauncher(t)
	st.share.Write("app/app.log", []byte(lines(1, 1)))
	// One session per share, as with sources on several shares of a server.
	for _, share := range []string{"a", "b", "c"} {
		st.sources.AddSource(newSMBSource(share, func(c *config.LogsConfig) { c.SMB.Share = share }))
	}
	st.out.waitLines(t, 3)
	require.Equal(t, 3, st.share.LiveSessions())

	st.share.SetLogoffLatency(time.Hour) // the server stopped answering
	returnsWithin(t, "Stop waited for the logoffs", st.launcher.Stop)
	assert.Zero(t, st.share.LiveSessions(), "the sessions were aborted")
	assert.Zero(t, st.share.Calls(fake.OpLogoff))
}

func TestLauncherSourceRemovalDoesNotWaitForTheLogoff(t *testing.T) {
	st := startLauncher(t)
	st.share.Write("app/app.log", []byte(lines(1, 1)))
	first := newSMBSource("first", func(c *config.LogsConfig) { c.SMB.Share = "a" })
	st.sources.AddSource(first)
	st.out.waitLines(t, 1)

	st.share.SetLogoffLatency(time.Hour) // the server stopped answering
	returnsWithin(t, "the source removal and the next addition waited for the logoff", func() {
		st.sources.RemoveSource(first)
		st.sources.AddSource(newSMBSource("second", func(c *config.LogsConfig) { c.SMB.Share = "b" }))
	})
	st.out.waitLines(t, 2)
	st.waitFor(t, func() bool { return st.share.Calls(fake.OpLogoff) == 1 }, "the removed source's session logs off in the background")

	returnsWithin(t, "Stop waited for the logoff", st.launcher.Stop)
	assert.Zero(t, st.share.LiveSessions(), "Stop cut the logoff short")
}

// TestLauncherReplacesASourceScheduledAgain covers a secret refresh:
// autodiscovery schedules a conf.d config again, with the new password, without
// removing the source it created before.
func TestLauncherReplacesASourceScheduledAgain(t *testing.T) {
	st := startLauncher(t)
	st.share.Write("app/app.log", []byte(lines(1, 2)))
	entry := func(password string) *sources.LogSource {
		return newSMBSource("demo", func(c *config.LogsConfig) {
			c.IntegrationSource = "file:/etc/datadog-agent/conf.d/demo.d/conf.yaml"
			c.IntegrationSourceIndex = 0
			c.SMB.Password = password
		})
	}
	previous := entry("old-key")
	st.sources.AddSource(previous)
	st.out.waitLines(t, 2)
	// Another entry of the same file is a different source.
	other := newSMBSource("demo", func(c *config.LogsConfig) {
		c.IntegrationSource = "file:/etc/datadog-agent/conf.d/demo.d/conf.yaml"
		c.IntegrationSourceIndex = 1
		c.Path = "app/other.log"
		c.SMB.Password = "other-key"
	})
	st.sources.AddSource(other)
	st.waitFor(t, func() bool { return other.Status().IsSuccess() }, "other entry scanned")
	require.Equal(t, 2, st.share.Calls(fake.OpDial))

	st.share.Append("app/app.log", []byte(lines(3, 3)))
	refreshed := entry("new-key")
	st.sources.AddSource(refreshed)
	st.waitFor(t, func() bool { return len(refreshed.GetInputs()) == 1 }, "the refreshed source tails the file")
	assert.Equal(t, []string{identifier("app/app.log")}, refreshed.GetInputs())
	assert.Empty(t, previous.GetInputs())
	assert.True(t, previous.IsHiddenFromStatus(), "agent status does not list the replaced source")
	assert.False(t, other.IsHiddenFromStatus())
	assert.Equal(t, 3, st.share.Calls(fake.OpDial), "the refreshed source dials with the new password")
	st.waitFor(t, func() bool { return st.share.LiveSessions() == 2 }, "the old password's session is closed")

	// A late removal of the replaced source does not touch the new one.
	st.sources.RemoveSource(previous)
	st.share.Append("app/app.log", []byte(lines(4, 4)))
	st.clock.Add(time.Second)
	st.out.waitLines(t, 4)

	st.launcher.Stop()
	st.out.flush()
	assert.Equal(t, want(1, 4), st.out.lines(), "nothing is read twice or skipped across the replacement")
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
