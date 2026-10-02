// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package softwareinventoryimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type captureCheckFunc func(types.ModuleName) ([]software.Entry, error)

func (f captureCheckFunc) GetCheck(module types.ModuleName) ([]software.Entry, error) {
	return f(module)
}

func captureManagerForTest(t *testing.T) *telemetrycapture.Manager {
	t.Helper()
	manager := telemetrycapture.NewManager("core", "test", "test")
	t.Cleanup(manager.Close)
	return manager
}

func waitForCaptureSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("software collection did not reach the expected boundary")
	}
}

func TestSoftwareCaptureDisabledHasNoCapability(t *testing.T) {
	f := newFixtureWithData(t, false, nil)
	f.reqs.CaptureManager = captureManagerForTest(t)
	is := f.sut()
	require.Empty(t, f.reqs.CaptureManager.Status().Capabilities)
	require.Zero(t, f.sysProbeClientAsMock().GetCallCount())
	is.captureReady(context.Background())
	require.Empty(t, f.reqs.CaptureManager.Status().Capabilities)
}

func TestSoftwareCaptureRequiresSuccessfulCollectorAndForwarder(t *testing.T) {
	for _, test := range []struct {
		name      string
		checkErr  error
		forwarder bool
	}{
		{name: "sysprobe unavailable", checkErr: sysprobeclient.ErrNotStartedYet, forwarder: true},
		{name: "collection failed", checkErr: errors.New("collection unavailable"), forwarder: true},
		{name: "forwarder unavailable", forwarder: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := captureManagerForTest(t)
			f := newFixtureWithData(t, false, nil)
			if !test.forwarder {
				f.reqs.EventPlatform = option.NonePtr[eventplatform.Forwarder]()
			}
			is := f.sut().softwareInventory
			is.enabled = true
			is.captureManager = manager
			is.interval = time.Hour
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			jitter := make(chan struct{})
			is.sleepFunc = func(time.Duration) { close(jitter) }
			lookedUp := make(chan struct{})
			is.sysProbeClient = captureCheckFunc(func(types.ModuleName) ([]software.Entry, error) {
				close(lookedUp)
				return []software.Entry{}, test.checkErr
			})
			go func() {
				defer close(done)
				is.startSoftwareInventoryCollection(ctx)
			}()
			waitForCaptureSignal(t, lookedUp)
			if !errors.Is(test.checkErr, sysprobeclient.ErrNotStartedYet) {
				waitForCaptureSignal(t, jitter)
			}
			require.Empty(t, manager.Status().Capabilities)
			cancel()
			waitForCaptureSignal(t, done)
			require.Empty(t, manager.Status().Capabilities)
		})
	}
}

func TestSoftwareCaptureReadinessCadenceAndStop(t *testing.T) {
	for _, interval := range []int{1, 43} {
		t.Run(time.Duration(interval).String(), func(t *testing.T) {
			manager := captureManagerForTest(t)
			f := newFixtureWithData(t, true, []software.Entry{})
			f.reqs.CaptureManager = manager
			f.reqs.Config.SetInTest("software_inventory.interval", interval)
			lc := f.reqs.Lc.(*compdef.TestLifecycle)
			defer func() { require.NoError(t, lc.Stop(context.Background())) }()
			is := f.sut().WaitForPayload()
			require.Equal(t, []telemetrycapture.Capability{{Stream: telemetrycapture.Software, Cadence: time.Duration(max(interval, 10)) * time.Minute}}, manager.Status().Capabilities)
			require.Equal(t, is.interval, manager.Status().Capabilities[0].Cadence)
			control := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "software-capture-session"}
			_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Software}})
			require.NoError(t, err)
			_, err = manager.Activate(control)
			require.NoError(t, err)
			require.NoError(t, lc.Stop(context.Background()))
			require.False(t, manager.Enabled())
			require.Empty(t, manager.Status().Capabilities)
			require.Equal(t, telemetrycapture.Failed, manager.Status().State)
		})
	}
}

func TestSoftwareCaptureLateCollectionCannotRestoreReadiness(t *testing.T) {
	manager := captureManagerForTest(t)
	f := newFixtureWithData(t, true, []software.Entry{})
	f.reqs.CaptureManager = manager
	lc := f.reqs.Lc.(*compdef.TestLifecycle)
	defer func() { require.NoError(t, lc.Stop(context.Background())) }()
	entered, release, jitter, finish := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var unblock, finishOnce sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(release) }); finishOnce.Do(func() { close(finish) }) })
	client := captureCheckFunc(func(types.ModuleName) ([]software.Entry, error) {
		close(entered)
		<-release
		return []software.Entry{}, nil
	})
	provides, err := newWithClient(f.reqs, client, func(time.Duration) {
		close(jitter)
		<-finish
	})
	require.NoError(t, err)
	is := provides.Comp.(*softwareInventory)
	waitForCaptureSignal(t, entered)
	require.Empty(t, manager.Status().Capabilities)
	require.NoError(t, lc.Stop(context.Background()))
	unblock.Do(func() { close(release) })
	waitForCaptureSignal(t, jitter)
	require.Empty(t, manager.Status().Capabilities, "late lookup completed but its readiness must stay stopped during jitter")
	is.captureReady(context.Background())
	require.Empty(t, manager.Status().Capabilities)
	finishOnce.Do(func() { close(finish) })
	require.Eventually(t, func() bool { return f.eventPlatformMock.GetCallCount() == 1 }, time.Second, time.Millisecond,
		"capture shutdown must preserve the existing initial submission")
}

func TestSoftwareCaptureCollectionFailureRemovesReadiness(t *testing.T) {
	manager := captureManagerForTest(t)
	f := newFixtureWithData(t, false, nil)
	is := f.sut().softwareInventory
	is.enabled = true
	is.captureManager = manager
	is.interval = time.Millisecond
	secondLookup, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	count := 0
	is.sysProbeClient = captureCheckFunc(func(types.ModuleName) ([]software.Entry, error) {
		count++
		if count == 1 {
			return []software.Entry{}, nil
		}
		if count == 2 {
			close(secondLookup)
			<-release
		}
		return nil, errors.New("collector unavailable")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); is.startSoftwareInventoryCollection(ctx) }()
	waitForCaptureSignal(t, secondLookup)
	require.Len(t, manager.Status().Capabilities, 1)
	unblock.Do(func() { close(release) })
	require.Eventually(t, func() bool { return len(manager.Status().Capabilities) == 0 }, time.Second, time.Millisecond)
	cancel()
	waitForCaptureSignal(t, done)
}
