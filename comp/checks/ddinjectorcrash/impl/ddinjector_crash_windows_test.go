// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjectorcrashimpl

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"golang.org/x/sys/windows"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	etw "github.com/DataDog/datadog-agent/comp/etw/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeETWComponent struct {
	newSessionCalls int
	session         *fakeETWSession
}

func (c *fakeETWComponent) NewSession(_ string, _ etw.SessionConfigurationFunc) (etw.Session, error) {
	c.newSessionCalls++
	return c.session, nil
}

func (c *fakeETWComponent) NewWellKnownSession(_ string, _ etw.SessionConfigurationFunc) (etw.Session, error) {
	return nil, nil
}

type fakeETWSession struct {
	stopOnce sync.Once
	stop     chan struct{}
	started  chan struct{}
	release  chan struct{}
}

func (s *fakeETWSession) ConfigureProvider(_ windows.GUID, configurations ...etw.ProviderConfigurationFunc) {
	for _, configure := range configurations {
		configure(&etw.ProviderConfiguration{})
	}
}

func (s *fakeETWSession) EnableProvider(_ windows.GUID) error  { return nil }
func (s *fakeETWSession) DisableProvider(_ windows.GUID) error { return nil }
func (s *fakeETWSession) StartTracing(_ etw.EventCallback) error {
	if s.started != nil {
		close(s.started)
	}
	<-s.stop
	if s.release != nil {
		<-s.release
	}
	return nil
}

func (s *fakeETWSession) StopTracing() error {
	s.stopOnce.Do(func() { close(s.stop) })
	return nil
}
func (s *fakeETWSession) GetSessionStatistics() (etw.SessionStatistics, error) {
	return etw.SessionStatistics{}, nil
}

func TestDDInjectorCrashListenerStopDeadline(t *testing.T) {
	config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"windows_crash_detection.enabled": true,
	})
	session := &fakeETWSession{
		stop: make(chan struct{}), started: make(chan struct{}), release: make(chan struct{}),
	}
	listener := newDDInjectorCrashListener(config, nil, &fakeETWComponent{session: session}, logmock.New(t))
	require.NoError(t, listener.start(context.Background()))
	<-session.started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, listener.stop(ctx), context.Canceled)
	// Tracing completes after Stop has returned; no private worker is left
	// waiting for Stop to close its queue.
	close(session.release)
	<-listener.traceDone
}

type fakeTelemetry struct {
	agenttelemetry.Component
	accept bool
	logs   []agenttelemetry.Log
}

func (a *fakeTelemetry) SubmitLog(log agenttelemetry.Log) bool {
	if !a.accept {
		return false
	}
	a.logs = append(a.logs, log)
	return true
}

func TestDDInjectorCrashListenerSubmitsAndCountsRejectedEvents(t *testing.T) {
	atel := &fakeTelemetry{}
	listener := newDDInjectorCrashListener(nil, atel, nil, logmock.New(t))
	data := crashUserData(postInjectionMessage, "crashy.exe", 42, 0xc0000005, 123)
	record := &etw.DDEventRecord{UserData: &data[0], UserDataLength: uint16(len(data))}
	record.EventHeader.EventDescriptor.Keyword = ddInjectorETWCrashKeyword
	listener.handleEvent(record)
	assert.Empty(t, atel.logs)
	atel.accept = true
	listener.handleEvent(record)
	require.Len(t, atel.logs, 1)
	log := atel.logs[0]
	assert.Equal(t, agenttelemetry.LogLevelError, log.Level)
	assert.Equal(t, ddInjectorCrashErrorKind, log.ErrorKind)
	assert.Equal(t, 1, log.Count)
	var event ddInjectorCrashEvent
	require.NoError(t, json.Unmarshal([]byte(log.Message), &event))
	assert.Equal(t, "crashy.exe", event.ProcessName)
	assert.Equal(t, uint32(42), event.ProcessID)
	assert.Equal(t, uint64(1), event.EventsSuppressed)
	assert.Zero(t, listener.suppressed.Load())
}

func TestDDInjectorCrashListenerEnabledOnlyByWindowsCrashDetection(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
			"injector.enable_telemetry":       true,
			"windows_crash_detection.enabled": false,
		})
		etwComponent := &fakeETWComponent{}
		listener := newDDInjectorCrashListener(config, nil, etwComponent, nil)

		require.NoError(t, listener.start(context.Background()))
		assert.Zero(t, etwComponent.newSessionCalls)
	})

	t.Run("enabled", func(t *testing.T) {
		config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
			"injector.enable_telemetry":       false,
			"windows_crash_detection.enabled": true,
		})
		session := &fakeETWSession{stop: make(chan struct{})}
		etwComponent := &fakeETWComponent{session: session}
		listener := newDDInjectorCrashListener(config, nil, etwComponent, logmock.New(t))

		require.NoError(t, listener.start(context.Background()))
		assert.Equal(t, 1, etwComponent.newSessionCalls)
		require.NoError(t, listener.stop(context.Background()))
	})
}
