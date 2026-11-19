// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package rcx509clientimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	remotequeriesv1alpha1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/agent_integrations/remote_queries"
)

const (
	wiringConn1 = "0194a1b2-c3d4-8e5f-a617-283940516273"
	wiringConn2 = "0194a1b2-ffff-8e5f-a617-283940516274"
)

// fakeCollector serves a fixed check list; every other method is unused.
type fakeCollector struct {
	collector.Component
	checks []check.Check
}

func (c fakeCollector) GetChecks() []check.Check { return c.checks }

func wiringConfig(t *testing.T, overrides map[string]interface{}) cfgcomp.Component {
	t.Helper()
	base := map[string]interface{}{
		"remote_queries.enabled":         true,
		executeEnabledSetting:            true,
		tunnelOrgIDSetting:               42,
		reportConnectionIDSetting:        true,
		"remote_configuration.enabled":   true,
		enabledSetting:                   true,
		"remote_configuration.api_key":   "key",
		"remote_configuration.rc_dd_url": "https://config.example.test",
	}
	for k, v := range overrides {
		base[k] = v
	}
	return cfgcomp.NewMockWithOverrides(t, base)
}

func wiringDeps(t *testing.T, overrides map[string]interface{}, c *fakeQueryCheck, tracker *ConnectionTracker) ExecuteHandlerDependencies {
	t.Helper()
	deps := ExecuteHandlerDependencies{Config: wiringConfig(t, overrides), Logger: logmock.New(t), Connections: tracker}
	if c != nil {
		deps.Collector = fakeCollector{checks: []check.Check{c}}
	}
	return deps
}

func TestExecuteHandlerIsNotProvidedByDefault(t *testing.T) {
	tracker := NewConnectionTracker()
	tracker.set(wiringConn1)
	c := &fakeQueryCheck{executed: make(chan string, 4)}
	for name, overrides := range map[string]map[string]interface{}{
		"default off":        {executeEnabledSetting: false},
		"off despite org id": {executeEnabledSetting: false, tunnelOrgIDSetting: 42},
	} {
		t.Run(name, func(t *testing.T) {
			require.Nil(t, NewExecuteHandler(wiringDeps(t, overrides, c, tracker)))
		})
	}

	// The unset configuration is the default: nothing is registered.
	t.Run("unset", func(t *testing.T) {
		deps := ExecuteHandlerDependencies{
			Config:      cfgcomp.NewMockWithOverrides(t, map[string]interface{}{"remote_queries.enabled": true}),
			Logger:      logmock.New(t),
			Connections: tracker,
			Collector:   fakeCollector{checks: []check.Check{c}},
		}
		require.False(t, deps.Config.GetBool(executeEnabledSetting))
		require.Zero(t, deps.Config.GetInt64(tunnelOrgIDSetting))
		require.Nil(t, NewExecuteHandler(deps))
	})

	// The component hands the factory no handler.
	var got rcx509client.ClientConfig
	d, _ := newDependencies(t, map[string]interface{}{enabledSetting: true}, func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		got = config
		return newFakeClient(), nil
	})
	New(d)
	require.Nil(t, got.ExecuteHandler)
}

func TestExecuteHandlerFailsClosedOnMissingPreconditions(t *testing.T) {
	c := &fakeQueryCheck{executed: make(chan string, 4)}
	tracker := NewConnectionTracker()
	for name, tc := range map[string]struct {
		overrides map[string]interface{}
		check     *fakeQueryCheck
		tracker   *ConnectionTracker
	}{
		"missing org":  {map[string]interface{}{tunnelOrgIDSetting: 0}, c, tracker},
		"negative org": {map[string]interface{}{tunnelOrgIDSetting: -1}, c, tracker},
		"reporter off": {map[string]interface{}{reportConnectionIDSetting: false}, c, tracker},
		"no collector": {nil, nil, tracker},
		"no tracker":   {nil, c, nil},
	} {
		t.Run(name, func(t *testing.T) {
			require.Nil(t, NewExecuteHandler(wiringDeps(t, tc.overrides, tc.check, tc.tracker)))
		})
	}
}

// With every precondition met the handler is provided, reaches the factory
// through the component, and the connection-ready line reports it.
func TestExecuteHandlerRegisteredWhenEnabled(t *testing.T) {
	tracker := NewConnectionTracker()
	c := &fakeQueryCheck{executed: make(chan string, 4)}
	handler := NewExecuteHandler(wiringDeps(t, nil, c, tracker))
	require.NotNil(t, handler)

	var got rcx509client.ClientConfig
	d, _ := newDependencies(t, map[string]interface{}{enabledSetting: true, reportConnectionIDSetting: true},
		func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
			got = config
			return newFakeClient(), nil
		})
	d.ExecuteHandler = handler
	New(d)
	require.NotNil(t, got.ExecuteHandler)
	require.True(t, got.ReportConnectionID)

	// The reporter that the factory installs feeds the tracker and reports the handler.
	logger := newRecordingLogger()
	r := newConnectionReporter(logger, nil)
	r.tracker = tracker
	r.markExecuteHandlerRegistered()
	r.handleLine(handshakeLine(wiringConn1))
	require.Equal(t, "remote queries tunnel connection ready connection_id="+wiringConn1+" execute_handler=true", logger.next(t))
	require.Equal(t, wiringConn1, tracker.Current())
	r.close()
	require.Empty(t, tracker.Current(), "a closed client has no live connection")
}

func TestTunnelAuthorityFailsClosedAndFollowsReconnect(t *testing.T) {
	tracker := NewConnectionTracker()
	conf := wiringConfig(t, nil)
	authority := tunnelAuthority{config: conf, connection: tracker.Current}

	// No handshake yet.
	_, err := authority.Authorize(context.Background(), 1)
	require.Error(t, err)

	tracker.set(wiringConn1)
	trust, err := authority.Authorize(context.Background(), 2)
	require.NoError(t, err)
	require.EqualValues(t, 42, trust.OrgID)
	require.Equal(t, wiringConn1, trust.ConnectionID)

	// A reconnect changes the ID that the authority vouches for.
	tracker.set(wiringConn2)
	trust, err = authority.Authorize(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, wiringConn2, trust.ConnectionID)

	// No org configured.
	_, err = tunnelAuthority{config: wiringConfig(t, map[string]interface{}{tunnelOrgIDSetting: 0}), connection: tracker.Current}.Authorize(context.Background(), 4)
	require.Error(t, err)
}

func wiringRequest(connection string, org int64) *remotequeriesv1alpha1.ExecuteRequest {
	request := executeRequest()
	request.ConnectionId = connection
	request.OrgId = org
	return request
}

// The full path: the Tunnel refuses before any handshake, accepts only a
// request naming the live connection and the configured org, and follows a
// reconnect. A refused request never reaches a check.
func TestExecuteHandlerEndToEndAuthorization(t *testing.T) {
	tracker := NewConnectionTracker()
	c := &fakeQueryCheck{executed: make(chan string, 4)}
	handler := NewExecuteHandler(wiringDeps(t, nil, c, tracker))
	require.NotNil(t, handler)

	// Before any handshake nothing is authorized, whatever the request says.
	response := call(t, handler, wiringRequest(wiringConn1, 42))
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
	require.Equal(t, "unauthorized", response.ErrorCode)
	require.Zero(t, c.resolves.Load())

	tracker.set(wiringConn1)
	for name, request := range map[string]*remotequeriesv1alpha1.ExecuteRequest{
		"another connection": wiringRequest(wiringConn2, 42),
		"another org":        wiringRequest(wiringConn1, 43),
		"no connection":      wiringRequest("", 42),
		"no org":             wiringRequest(wiringConn1, 0),
	} {
		t.Run(name, func(t *testing.T) {
			response := call(t, handler, request)
			require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
			require.Equal(t, "unauthorized", response.ErrorCode)
		})
	}
	require.Zero(t, c.resolves.Load(), "a refused request must not reach a check")
	require.Zero(t, c.executes.Load())

	response = call(t, handler, wiringRequest(wiringConn1, 42))
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ACCEPTED, response.Status)
	select {
	case <-c.executed:
	case <-time.After(3 * time.Second):
		t.Fatal("execute did not start")
	}

	// After a reconnect the old ID is refused and the new one is served.
	tracker.set(wiringConn2)
	stale := wiringRequest(wiringConn1, 42)
	stale.RunId = "0e3c8f1e-5f54-4f0b-9a52-6c1d2b3a4f5e"
	response = call(t, handler, stale)
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_REJECTED, response.Status)
	require.Equal(t, "unauthorized", response.ErrorCode)
	fresh := wiringRequest(wiringConn2, 42)
	fresh.RunId = "0e3c8f1e-5f54-4f0b-9a52-6c1d2b3a4f5f"
	response = call(t, handler, fresh)
	require.Equal(t, remotequeriesv1alpha1.ExecuteStatus_EXECUTE_STATUS_ACCEPTED, response.Status)
	select {
	case <-c.executed:
	case <-time.After(3 * time.Second):
		t.Fatal("execute did not start after the reconnect")
	}
}
