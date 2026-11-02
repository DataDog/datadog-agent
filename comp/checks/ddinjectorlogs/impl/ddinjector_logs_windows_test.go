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
	"time"

	"golang.org/x/sys/windows"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	etw "github.com/DataDog/datadog-agent/comp/etw/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
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
		"injector.enable_logs": true,
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
	listener.driverVersion = func() (string, error) { return "1.2.3.4", nil }
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
	assert.Equal(t, "ddinjector: CrashAttribution_Event/post_injection", log.ErrorKind)
	assert.Equal(t, "component:ddinjector,agent.flavor:"+flavor.GetFlavor()+",ddinjector.version:1.2.3.4", log.Tags)
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

func TestDDInjectorLogsListenerErrorKindSplitsCrashAttribution(t *testing.T) {
	for _, tc := range []struct {
		message string
		kind    string
	}{
		{"Injection-related crash during injection detected", "ddinjector: CrashAttribution_Event/during_injection"},
		{"Injection-related crash post injection detected", "ddinjector: CrashAttribution_Event/post_injection"},
		{"something else", "ddinjector: CrashAttribution_Event"},
	} {
		atel := &fakeTelemetry{accept: true}
		listener, _ := newTestListener(t, atel, map[string]interface{}{"Message": tc.message}, nil)
		listener.handleEvent(tlEventRecord("CrashAttribution_Event"))
		require.Len(t, atel.logs, 1)
		assert.Equal(t, tc.kind, atel.logs[0].ErrorKind, tc.message)
	}
}

func TestDDInjectorLogsListenerRefreshesDriverVersion(t *testing.T) {
	atel := &fakeTelemetry{accept: true}
	listener, _ := newTestListener(t, atel, nil, nil)
	now := time.Unix(1000, 0)
	listener.now = func() time.Time { return now }
	versions := []struct {
		version string
		err     error
	}{
		{"", errors.New("not installed")},
		{"1.2.3.4", nil},
		{"", errors.New("transient")},
		{"1.2.3.5", nil},
	}
	versionCalls := 0
	listener.driverVersion = func() (string, error) {
		v := versions[versionCalls]
		versionCalls++
		return v.version, v.err
	}
	record := tlEventRecord("CrashAttribution_Event")
	baseTags := "component:ddinjector,agent.flavor:" + flavor.GetFlavor()

	// A failed read is retried on the next event.
	listener.handleEvent(record)
	listener.handleEvent(record)
	// Within the refresh interval, the version is not read again.
	now = now.Add(ddInjectorVersionRefreshInterval - time.Second)
	listener.handleEvent(record)
	// Once the interval elapsed, a failed refresh keeps the last known version...
	now = now.Add(2 * time.Second)
	listener.handleEvent(record)
	// ...and the next event picks up the new one.
	listener.handleEvent(record)

	require.Len(t, atel.logs, 5)
	assert.Equal(t, baseTags, atel.logs[0].Tags)
	assert.Equal(t, baseTags+",ddinjector.version:1.2.3.4", atel.logs[1].Tags)
	assert.Equal(t, baseTags+",ddinjector.version:1.2.3.4", atel.logs[2].Tags)
	assert.Equal(t, baseTags+",ddinjector.version:1.2.3.4", atel.logs[3].Tags)
	assert.Equal(t, baseTags+",ddinjector.version:1.2.3.5", atel.logs[4].Tags)
	assert.Equal(t, 4, versionCalls)
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
		"injector.enable_logs": true,
	})
	session := &fakeETWSession{stop: make(chan struct{})}
	listener := newDDInjectorLogsListener(config, nil, &fakeETWComponent{session: session}, logmock.New(t))

	require.NoError(t, listener.start(context.Background()))
	require.NoError(t, listener.stop(context.Background()))
	assert.Equal(t, etw.TRACE_LEVEL_WARNING, session.provider.TraceLevel)
	assert.Equal(t, uint64(0x40), session.provider.MatchAnyKeyword)
}

func TestDDInjectorLogsListenerEnabledOnlyByEnableLogs(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
			"injector.enable_telemetry":       true,
			"windows_crash_detection.enabled": true,
			"injector.enable_logs":            false,
		})
		etwComponent := &fakeETWComponent{}
		listener := newDDInjectorLogsListener(config, nil, etwComponent, nil)

		require.NoError(t, listener.start(context.Background()))
		assert.Zero(t, etwComponent.newSessionCalls)
	})

	t.Run("enabled", func(t *testing.T) {
		config := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
			"injector.enable_telemetry":       false,
			"windows_crash_detection.enabled": false,
			"injector.enable_logs":            true,
		})
		session := &fakeETWSession{stop: make(chan struct{})}
		etwComponent := &fakeETWComponent{session: session}
		listener := newDDInjectorLogsListener(config, nil, etwComponent, logmock.New(t))

		require.NoError(t, listener.start(context.Background()))
		assert.Equal(t, 1, etwComponent.newSessionCalls)
		require.NoError(t, listener.stop(context.Background()))
	})
}
