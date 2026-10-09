// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjectorlogsimpl

import (
	"context"
	"encoding/json"
	"errors"
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
	provider etw.ProviderConfiguration
}

func (s *fakeETWSession) ConfigureProvider(_ windows.GUID, configurations ...etw.ProviderConfigurationFunc) {
	for _, configure := range configurations {
		configure(&s.provider)
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

func TestDDInjectorLogsListenerStopDeadline(t *testing.T) {
	config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"windows_crash_detection.enabled": true,
	})
	session := &fakeETWSession{
		stop: make(chan struct{}), started: make(chan struct{}), release: make(chan struct{}),
	}
	listener := newDDInjectorLogsListener(config, nil, &fakeETWComponent{session: session}, logmock.New(t))
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

// tlEventRecord builds an event record carrying TraceLogging schema metadata for name.
func tlEventRecord(name string) *etw.DDEventRecord {
	metadata := traceLoggingMetadata([]byte{0}, name, nil)
	item := &etw.DDEventHeaderExtendedDataItem{
		ExtType:  eventHeaderExtTypeEventSchemaTL,
		DataSize: uint16(len(metadata)),
		DataPtr:  &metadata[0],
	}
	record := &etw.DDEventRecord{ExtendedData: item, ExtendedDataCount: 1}
	record.EventHeader.EventDescriptor.Level = uint8(etw.TRACE_LEVEL_WARNING)
	record.EventHeader.EventDescriptor.Keyword = 0x40
	return record
}

func newTestListener(t *testing.T, atel agenttelemetry.Component, properties map[string]interface{}, decodeErr error) (*ddInjectorLogsListener, *int) {
	listener := newDDInjectorLogsListener(nil, atel, nil, logmock.New(t))
	decodeCalls := 0
	listener.decodeProperties = func(*etw.DDEventRecord) (map[string]interface{}, error) {
		decodeCalls++
		return properties, decodeErr
	}
	return listener, &decodeCalls
}

func TestDDInjectorLogsListenerForwardsAllPropertiesAndCountsRejectedEvents(t *testing.T) {
	atel := &fakeTelemetry{}
	properties := map[string]interface{}{
		"Message":     "Injection-related crash post injection detected",
		"ProcessName": `\Device\HarddiskVolume3\crashy.exe`,
		"ProcessId":   "42",
		"ExitStatus":  "0xC0000005",
		"ElapsedMs":   "123",
	}
	listener, _ := newTestListener(t, atel, properties, nil)
	record := tlEventRecord("CrashAttribution_Event")

	listener.handleEvent(record)
	assert.Empty(t, atel.logs)
	atel.accept = true
	listener.handleEvent(record)

	require.Len(t, atel.logs, 1)
	log := atel.logs[0]
	assert.Equal(t, agenttelemetry.LogLevelError, log.Level)
	assert.Equal(t, "ddinjector_crash", log.ErrorKind)
	assert.Equal(t, 1, log.Count)
	var event ddInjectorLog
	require.NoError(t, json.Unmarshal([]byte(log.Message), &event))
	assert.Equal(t, ddInjectorLog{
		Event:            "CrashAttribution_Event",
		Level:            uint8(etw.TRACE_LEVEL_WARNING),
		Keyword:          0x40,
		Properties:       properties,
		EventsSuppressed: 1,
	}, event)
	assert.Zero(t, listener.forwarders["CrashAttribution_Event"].suppressed.Load())
}

func TestDDInjectorLogsListenerForwardsPartialPropertiesOnDecodeError(t *testing.T) {
	atel := &fakeTelemetry{accept: true}
	listener, _ := newTestListener(t, atel, map[string]interface{}{"Message": "partial"}, errors.New("bad property"))
	record := tlEventRecord("CrashAttribution_Event")

	listener.handleEvent(record)

	require.Len(t, atel.logs, 1)
	var event ddInjectorLog
	require.NoError(t, json.Unmarshal([]byte(atel.logs[0].Message), &event))
	assert.Equal(t, map[string]interface{}{"Message": "partial"}, event.Properties)
	assert.Equal(t, "bad property", event.DecodeError)
}

func TestDDInjectorLogsListenerIgnoresUnwatchedEvents(t *testing.T) {
	atel := &fakeTelemetry{accept: true}
	listener, decodeCalls := newTestListener(t, atel, nil, nil)

	listener.handleEvent(tlEventRecord("Ioctl_CreateClose_Request"))
	listener.handleEvent(&etw.DDEventRecord{})

	assert.Empty(t, atel.logs)
	assert.Zero(t, *decodeCalls, "unwatched events must not be decoded")
}

func TestDDInjectorLogsListenerEnablesWatchedKeywords(t *testing.T) {
	config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"windows_crash_detection.enabled": true,
	})
	session := &fakeETWSession{stop: make(chan struct{})}
	listener := newDDInjectorLogsListener(config, nil, &fakeETWComponent{session: session}, logmock.New(t))

	require.NoError(t, listener.start(context.Background()))
	require.NoError(t, listener.stop(context.Background()))
	assert.Equal(t, etw.TRACE_LEVEL_WARNING, session.provider.TraceLevel)
	assert.Equal(t, uint64(0x40), session.provider.MatchAnyKeyword)
}

func TestDDInjectorLogsListenerEnabledOnlyByWindowsCrashDetection(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
			"injector.enable_telemetry":       true,
			"windows_crash_detection.enabled": false,
		})
		etwComponent := &fakeETWComponent{}
		listener := newDDInjectorLogsListener(config, nil, etwComponent, nil)

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
		listener := newDDInjectorLogsListener(config, nil, etwComponent, logmock.New(t))

		require.NoError(t, listener.start(context.Background()))
		assert.Equal(t, 1, etwComponent.newSessionCalls)
		require.NoError(t, listener.stop(context.Background()))
	})
}
