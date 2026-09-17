// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
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
// networkdevices.Component.
type fakeNetworkDevices struct {
	*fakeChecker
}

func (fakeNetworkDevices) ConnectivityCheckEndpointHandler() http.HandlerFunc { return nil }

// testRequires builds the component dependencies from a mock config.
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
	reqs, lc := testRequires(t, config.NewMock(t))
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	assert.Equal(t, 0, provides.Comp.RangeCount())
	lc.AssertHooksNumber(0)
}

func TestNewComponentEnabledRegistersItsLifecycleHook(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
	})

	reqs, lc := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	assert.Equal(t, 0, provides.Comp.RangeCount())
	lc.AssertHooksNumber(1)
}

func TestNewComponentFailsWithoutTheEventPlatformForwarder(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
	})

	reqs, _ := testRequires(t, cfg)
	reqs.EventPlatform = fakeEventPlatform{}

	_, err := NewComponent(reqs)
	require.Error(t, err)
}

func TestNewComponentUsesOneWorkerNumberEverywhere(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
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
	assert.Equal(t, rangeDefaults{Namespace: "lab", IntervalSec: 900, MaxAddresses: 1024}, comp.defaults)
}

func TestStartProbesPingOnceAndStops(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
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

func newScheduleTestComponent(t *testing.T) (ndmdiscovery.Component, *compdef.TestLifecycle) {
	t.Helper()
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"network_devices.discovery.enabled": true,
		"network_devices.snmp_credentials": []interface{}{
			map[string]interface{}{"id": "cred-a", "snmp_version": "2c", "community_string": "public"},
		},
	})

	reqs, lc := testRequires(t, cfg)
	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	require.NoError(t, lc.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, lc.Stop(context.Background())) })
	return provides.Comp, lc
}

func TestScheduleStopsARangeAbsentFromTheSnapshot(t *testing.T) {
	comp, _ := newScheduleTestComponent(t)

	errs := comp.Schedule([]ndmdiscovery.Range{
		{ID: "ad-1", CIDR: "10.0.0.0/24", CredentialIDs: []string{"cred-a"}},
		{ID: "ad-2", CIDR: "10.0.1.0/24", CredentialIDs: []string{"cred-a"}},
	})
	assert.Empty(t, errs)
	assert.Equal(t, 2, comp.RangeCount())

	errs = comp.Schedule([]ndmdiscovery.Range{
		{ID: "ad-1", CIDR: "10.0.0.0/24", CredentialIDs: []string{"cred-a"}},
	})
	assert.Empty(t, errs)
	assert.Equal(t, 1, comp.RangeCount())

	assert.Empty(t, comp.Schedule(nil))
	assert.Equal(t, 0, comp.RangeCount())
}

func TestScheduleReportsOneErrorPerRejectedRange(t *testing.T) {
	comp, _ := newScheduleTestComponent(t)

	errs := comp.Schedule([]ndmdiscovery.Range{
		{ID: "good", CIDR: "10.0.0.0/24", CredentialIDs: []string{"cred-a"}},
		{ID: "bad-cidr", CIDR: "nope", CredentialIDs: []string{"cred-a"}},
		{ID: "unknown-cred", CIDR: "10.0.2.0/24", CredentialIDs: []string{"cred-z"}},
	})

	require.Len(t, errs, 2)
	assert.Contains(t, errs["bad-cidr"].Error(), "invalid CIDR")
	assert.Contains(t, errs["unknown-cred"].Error(), "cred-z")
	assert.Equal(t, 1, comp.RangeCount(), "a rejected range does not stop the others")
}

func TestScheduleOnADisabledComponentRejectsEveryRange(t *testing.T) {
	reqs, _ := testRequires(t, config.NewMock(t))
	provides, err := NewComponent(reqs)
	require.NoError(t, err)

	errs := provides.Comp.Schedule([]ndmdiscovery.Range{{ID: "ad-1", CIDR: "10.0.0.0/24", CredentialIDs: []string{"cred-a"}}})
	require.Len(t, errs, 1)
	assert.ErrorIs(t, errs["ad-1"], errDisabled)
}
