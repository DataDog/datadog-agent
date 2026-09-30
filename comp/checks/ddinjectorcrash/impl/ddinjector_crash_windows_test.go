// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjectorcrashimpl

import (
	"context"
	"sync"
	"testing"

	"golang.org/x/sys/windows"

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
}

func (s *fakeETWSession) ConfigureProvider(_ windows.GUID, configurations ...etw.ProviderConfigurationFunc) {
	for _, configure := range configurations {
		configure(&etw.ProviderConfiguration{})
	}
}

func (s *fakeETWSession) EnableProvider(_ windows.GUID) error  { return nil }
func (s *fakeETWSession) DisableProvider(_ windows.GUID) error { return nil }
func (s *fakeETWSession) StartTracing(_ etw.EventCallback) error {
	<-s.stop
	return nil
}
func (s *fakeETWSession) StopTracing() error {
	s.stopOnce.Do(func() { close(s.stop) })
	return nil
}
func (s *fakeETWSession) GetSessionStatistics() (etw.SessionStatistics, error) {
	return etw.SessionStatistics{}, nil
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
