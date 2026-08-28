// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// fakeForwarder adapts fakeSender to the event platform Forwarder interface.
type fakeForwarder struct {
	*fakeSender
}

func (f fakeForwarder) SendEventPlatformEvent(m *message.Message, eventType string) error {
	return f.SendEventPlatformEventBlocking(m, eventType)
}

func (f fakeForwarder) Purge() map[string][]*message.Message { return nil }

// fakeEventPlatform is an eventplatform.Component whose forwarder is always
// available.
type fakeEventPlatform struct {
	forwarder eventplatform.Forwarder
}

func (f fakeEventPlatform) Get() (eventplatform.Forwarder, bool) {
	return f.forwarder, f.forwarder != nil
}

// fakeNetworkDevices presents a scripted fakeChecker as the full
// networkdevices.Component. Only CheckConnectivity is exercised: the
// component consumes the dependency through the narrower connectivityChecker
// interface.
type fakeNetworkDevices struct {
	*fakeChecker
}

func (fakeNetworkDevices) ConnectivityCheckEndpointHandler() http.HandlerFunc { return nil }

// testRequires builds the component dependencies from a mock config, a mock
// logger, a lifecycle spy, a scripted connectivity checker and a recording
// event platform forwarder.
func testRequires(t *testing.T, cfg config.Component) (Requires, *compdef.TestLifecycle) {
	t.Helper()
	lc := compdef.NewTestLifecycle(t)
	return Requires{
		Lifecycle:      lc,
		Log:            logmock.New(t),
		Config:         cfg,
		EventPlatform:  fakeEventPlatform{forwarder: fakeForwarder{&fakeSender{}}},
		NetworkDevices: fakeNetworkDevices{&fakeChecker{}},
	}, lc
}

func TestNewComponentDisabledByDefault(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"remote_configuration.enabled": true,
	})

	reqs, lc := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	assert.Nil(t, provides.RCListener.ListenerProvider,
		"a disabled component must not subscribe to remote config")
	assert.Equal(t, 0, provides.Comp.RangeCount())
	lc.AssertHooksNumber(0)
}

func TestNewComponentDisabledWhenRemoteConfigOff(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
		"remote_configuration.enabled":      false,
	})

	reqs, lc := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	assert.Nil(t, provides.RCListener.ListenerProvider)
	assert.Equal(t, 0, provides.Comp.RangeCount())
	lc.AssertHooksNumber(0)
}

func TestNewComponentEnabledSubscribesToTheSharedProduct(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
		"remote_configuration.enabled":      true,
	})

	reqs, lc := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	require.NotNil(t, provides.RCListener.ListenerProvider)
	_, ok := provides.RCListener.ListenerProvider[data.ProductManagedDeploymentsDebug]
	assert.True(t, ok, "the component subscribes to the shared POC product")
	assert.Len(t, provides.RCListener.ListenerProvider, 1)
	assert.Equal(t, 0, provides.Comp.RangeCount())
	lc.AssertHooksNumber(1)
}

func TestNewComponentFailsWithoutTheEventPlatformForwarder(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
		"remote_configuration.enabled":      true,
	})

	reqs, _ := testRequires(t, cfg)
	reqs.EventPlatform = fakeEventPlatform{}

	_, err := NewComponent(reqs)
	require.Error(t, err)
}

// The semaphore's size and the sweeper's budget are never cross-checked at
// run time: a budget larger than the semaphore makes every Acquire block until
// the context is cancelled, silently stalling every sweep with no error and no
// log line. This pins all three worker numbers to the single configured value.
func TestNewComponentUsesOneWorkerNumberEverywhere(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
		"remote_configuration.enabled":      true,
		"network_devices.discovery.workers": 3,
	})

	reqs, _ := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	comp, ok := provides.Comp.(*ndmDiscovery)
	require.True(t, ok)

	assert.Equal(t, int64(3), comp.sched.opts.Workers, "scheduler worker budget")
	assert.Equal(t, int64(3), comp.sched.sweeper.budget, "sweeper budget")

	sem := comp.sched.sweeper.sem
	require.True(t, sem.TryAcquire(3), "the semaphore must grant the whole budget")
	sem.Release(3)
	assert.False(t, sem.TryAcquire(4), "the semaphore must be no larger than the budget")
}

func TestNewComponentClampsANonPositiveWorkerCount(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
		"remote_configuration.enabled":      true,
		"network_devices.discovery.workers": 0,
	})

	reqs, _ := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	comp := provides.Comp.(*ndmDiscovery)
	assert.Equal(t, int64(1), comp.sched.opts.Workers)
	assert.Equal(t, int64(1), comp.sched.sweeper.budget)
	require.True(t, comp.sched.sweeper.sem.TryAcquire(1))
	assert.False(t, comp.sched.sweeper.sem.TryAcquire(1))
}

func TestNewComponentReadsTheRangeDefaults(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled":              true,
		"remote_configuration.enabled":                   true,
		"network_devices.discovery.default_namespace":    "lab",
		"network_devices.discovery.default_interval_sec": 900,
		"network_devices.discovery.max_range_addresses":  1024,
	})

	reqs, _ := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	comp := provides.Comp.(*ndmDiscovery)
	assert.Equal(t, rangeDefaults{Namespace: "lab", IntervalSec: 900, MaxAddresses: 1024}, comp.sched.opts.Defaults)
	assert.Equal(t, 1024, comp.sched.opts.MaxAddresses)
	assert.Equal(t, rangeDefaults{Namespace: "lab", IntervalSec: 900, MaxAddresses: 1024}, comp.rc.defaults)
}

// The lifecycle hooks must run the ping probe exactly once and leave ping
// disabled when this agent cannot send ICMP.
func TestStartProbesPingOnceAndStops(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
		"remote_configuration.enabled":      true,
	})

	reqs, lc := testRequires(t, cfg)
	checker := &fakeChecker{respond: func(_ connectivity.Request) (connectivity.Result, error) {
		return connectivity.Result{}, nil
	}}
	reqs.NetworkDevices = fakeNetworkDevices{checker}
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	require.NoError(t, lc.Start(t.Context()))
	comp := provides.Comp.(*ndmDiscovery)
	comp.sched.mu.Lock()
	pingEnabled := comp.sched.pingEnabled
	comp.sched.mu.Unlock()
	assert.False(t, pingEnabled, "a ping-incapable agent leaves ping disabled")
	assert.Len(t, checker.recorded(), 1, "ping is probed exactly once, at start")

	require.NoError(t, lc.Stop(t.Context()))
}
