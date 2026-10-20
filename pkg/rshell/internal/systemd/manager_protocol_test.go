// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeManagerCall struct {
	destination string
	path        dbus.ObjectPath
	method      string
	arguments   []any
}

type fakeManagerBus struct {
	calls      []fakeManagerCall
	respond    func(fakeManagerCall) ([]any, error)
	matchErr   error
	signalSink chan<- *dbus.Signal
	overflow   chan struct{}
}

func (b *fakeManagerBus) call(_ context.Context, destination string, path dbus.ObjectPath, method string, arguments ...any) ([]any, error) {
	call := fakeManagerCall{destination: destination, path: path, method: method, arguments: append([]any(nil), arguments...)}
	b.calls = append(b.calls, call)
	if b.respond == nil {
		return nil, nil
	}
	return b.respond(call)
}

func (b *fakeManagerBus) addJobRemovedMatch(context.Context) error {
	return b.matchErr
}

func (b *fakeManagerBus) registerSignals(channel chan<- *dbus.Signal) <-chan struct{} {
	b.signalSink = channel
	if b.overflow == nil {
		b.overflow = make(chan struct{})
	}
	return b.overflow
}

func (b *fakeManagerBus) removeSignals(channel chan<- *dbus.Signal) {
	if b.signalSink == channel {
		b.signalSink = nil
	}
}

func TestVerifySystemdManagerMachineIDUsesManagerPeer(t *testing.T) {
	expectedMachineID := "0123456789abcdef0123456789abcdef"
	bus := &fakeManagerBus{respond: func(call fakeManagerCall) ([]any, error) {
		assert.Equal(t, "org.freedesktop.systemd1", call.destination)
		assert.Equal(t, dbus.ObjectPath("/org/freedesktop/systemd1"), call.path)
		assert.Equal(t, "org.freedesktop.DBus.Peer.GetMachineId", call.method)
		assert.Empty(t, call.arguments)
		return []any{strings.ToUpper(expectedMachineID)}, nil
	}}

	require.NoError(t, verifySystemdManagerMachineID(context.Background(), bus, expectedMachineID))
	require.Len(t, bus.calls, 1)
}

func TestVerifySystemdManagerMachineIDRejectsMismatch(t *testing.T) {
	bus := &fakeManagerBus{respond: func(fakeManagerCall) ([]any, error) {
		return []any{"fedcba9876543210fedcba9876543210"}, nil
	}}

	err := verifySystemdManagerMachineID(context.Background(), bus, "0123456789abcdef0123456789abcdef")
	require.EqualError(t, err, "systemd manager peer machine ID does not match the configured target")
}

type fakeManagerUnit struct {
	selector      string
	canonical     string
	description   string
	loadState     string
	activeState   string
	subState      string
	unitFileState string
	jobID         uint32
	mainPID       uint32
	result        string
}

func fakeManagerStateBus(units ...fakeManagerUnit) *fakeManagerBus {
	bySelector := make(map[string]fakeManagerUnit, len(units))
	byPath := make(map[dbus.ObjectPath]fakeManagerUnit, len(units))
	for index, unit := range units {
		bySelector[unit.selector] = unit
		byPath[dbus.ObjectPath(fmt.Sprintf("%sunit_%d", systemdUnitPathPrefix, index+1))] = unit
	}
	bus := &fakeManagerBus{}
	bus.respond = func(call fakeManagerCall) ([]any, error) {
		switch call.method {
		case systemdManagerIface + ".GetUnit", systemdManagerIface + ".LoadUnit":
			selector := call.arguments[0].(string)
			unit, found := bySelector[selector]
			if !found {
				return nil, dbus.Error{Name: "org.freedesktop.systemd1.NoSuchUnit"}
			}
			for path, candidate := range byPath {
				if candidate.selector == unit.selector {
					return []any{path}, nil
				}
			}
		case dbusPropertiesGet:
			unit, found := byPath[call.path]
			if !found {
				return nil, errors.New("unexpected unit object path")
			}
			property := call.arguments[1].(string)
			var value any
			switch property {
			case "Id":
				value = unit.canonical
			case "Description":
				value = unit.description
			case "LoadState":
				value = unit.loadState
			case "ActiveState":
				value = unit.activeState
			case "SubState":
				value = unit.subState
			case "UnitFileState":
				value = unit.unitFileState
			case "Job":
				path := dbus.ObjectPath("/")
				if unit.jobID != 0 {
					path = dbus.ObjectPath(fmt.Sprintf("%s%d", systemdJobPathPrefix, unit.jobID))
				}
				value = unitJobProperty{ID: unit.jobID, Path: path}
			case "MainPID":
				value = unit.mainPID
			case "Result":
				value = unit.result
			default:
				return nil, fmt.Errorf("unexpected property %q", property)
			}
			return []any{dbus.MakeVariant(value)}, nil
		}
		return nil, fmt.Errorf("unexpected manager method %q", call.method)
	}
	return bus
}

func TestListSystemServicesUsesOnlyFixedMinimalPropertiesAndFiltersInactive(t *testing.T) {
	bus := fakeManagerStateBus(
		fakeManagerUnit{selector: "active.service", canonical: "active.service", description: "active", loadState: "loaded", activeState: "active", subState: "running"},
		fakeManagerUnit{selector: "idle.timer", canonical: "idle.timer", description: "idle", loadState: "loaded", activeState: "inactive", subState: "dead"},
		fakeManagerUnit{selector: "queued.socket", canonical: "queued.socket", description: "queued", loadState: "loaded", activeState: "inactive", subState: "dead", jobID: 42},
	)

	states, err := listSystemServicesWithBus(context.Background(), bus, builtins.SystemServiceListRequest{
		Services: []string{"active.service", "idle.timer", "queued.socket", "missing.path"},
	})
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, "active.service", states[0].Name)
	assert.Equal(t, "queued.socket", states[1].Name)
	assert.Equal(t, uint32(42), states[1].JobID)

	for _, call := range bus.calls {
		if call.method != dbusPropertiesGet {
			continue
		}
		property := call.arguments[1].(string)
		assert.Contains(t, []string{"Id", "Description", "LoadState", "ActiveState", "SubState", "Job"}, property)
	}
}

func TestListSystemServicesAllLoadsInactiveAndPreservesAuthorizedAlias(t *testing.T) {
	bus := fakeManagerStateBus(fakeManagerUnit{
		selector:    "alias.timer",
		canonical:   `real\x2dname:tenant.timer`,
		description: "timer",
		loadState:   "loaded",
		activeState: "inactive",
		subState:    "dead",
	})
	states, err := listSystemServicesWithBus(context.Background(), bus, builtins.SystemServiceListRequest{
		Services:        []string{"alias.timer", "missing.timer"},
		IncludeInactive: true,
	})
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "alias.timer", states[0].Name)
	assert.Equal(t, `real\x2dname:tenant.timer`, states[0].CanonicalName)
	assert.Equal(t, systemdManagerIface+".LoadUnit", bus.calls[0].method)
}

func TestInspectSystemServicesReturnsDetailsAndSynthesizesNotFound(t *testing.T) {
	bus := fakeManagerStateBus(fakeManagerUnit{
		selector:      "api.service",
		canonical:     "api.service",
		description:   "API",
		loadState:     "loaded",
		activeState:   "failed",
		subState:      "failed",
		unitFileState: "enabled",
		mainPID:       123,
		result:        "exit-code",
	})
	states, err := inspectSystemServicesWithBus(context.Background(), bus, []string{"api.service", "missing.service"})
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, uint32(123), states[0].MainPID)
	assert.Equal(t, "enabled", states[0].UnitFileState)
	assert.Equal(t, builtins.SystemServiceState{
		Name:        "missing.service",
		LoadState:   "not-found",
		ActiveState: "inactive",
		SubState:    "dead",
	}, states[1])
	var properties []string
	for _, call := range bus.calls {
		if call.method == dbusPropertiesGet {
			properties = append(properties, call.arguments[1].(string))
		}
	}
	assert.Equal(t, []string{"Id", "Description", "LoadState", "ActiveState", "SubState", "UnitFileState", "Result", "MainPID"}, properties)
}

func TestInspectSystemServicesUsesTypeSpecificResultInterface(t *testing.T) {
	bus := fakeManagerStateBus(
		fakeManagerUnit{selector: "backup.timer", canonical: "backup.timer", loadState: "loaded", activeState: "inactive", subState: "dead", result: "success"},
		fakeManagerUnit{selector: "backup.target", canonical: "backup.target", loadState: "loaded", activeState: "active", subState: "active"},
	)
	states, err := inspectSystemServicesWithBus(context.Background(), bus, []string{"backup.timer", "backup.target"})
	require.NoError(t, err)
	require.Len(t, states, 2)
	assert.Equal(t, "success", states[0].Result)
	assert.Empty(t, states[1].Result)

	var resultCalls []fakeManagerCall
	for _, call := range bus.calls {
		if call.method == dbusPropertiesGet && call.arguments[1] == "Result" {
			resultCalls = append(resultCalls, call)
		}
	}
	require.Len(t, resultCalls, 1)
	assert.Equal(t, "org.freedesktop.systemd1.Timer", resultCalls[0].arguments[0])
}

func TestManagerResultInterfaceIsFixedBySupportedUnitType(t *testing.T) {
	tests := map[string]string{
		"api.service":       "org.freedesktop.systemd1.Service",
		"api.socket":        "org.freedesktop.systemd1.Socket",
		"data.mount":        "org.freedesktop.systemd1.Mount",
		"data.automount":    "org.freedesktop.systemd1.Automount",
		"backup.timer":      "org.freedesktop.systemd1.Timer",
		"scratch.swap":      "org.freedesktop.systemd1.Swap",
		"incoming.path":     "org.freedesktop.systemd1.Path",
		"session-1.scope":   "org.freedesktop.systemd1.Scope",
		"multi-user.target": "",
		"dev-null.device":   "",
		"system.slice":      "",
	}
	for unit, expected := range tests {
		actual, ok := managerResultInterface(unit)
		assert.Equal(t, expected != "", ok, unit)
		assert.Equal(t, expected, actual, unit)
	}
}

func TestRunSystemServiceJobsUsesFixedMethodAndWaitsForMatchingResult(t *testing.T) {
	bus := &fakeManagerBus{}
	bus.respond = func(call fakeManagerCall) ([]any, error) {
		switch call.method {
		case systemdManagerIface + ".Subscribe":
			return nil, nil
		case systemdManagerIface + ".ReloadUnit":
			require.Equal(t, []any{"api.service", "replace"}, call.arguments)
			bus.signalSink <- &dbus.Signal{
				Path: systemdManagerPath,
				Name: systemdManagerIface + ".JobRemoved",
				Body: []any{uint32(77), dbus.ObjectPath(systemdJobPathPrefix + "77"), `real\x2dapi.service`, "done"},
			}
			return []any{dbus.ObjectPath(systemdJobPathPrefix + "77")}, nil
		default:
			return nil, fmt.Errorf("unexpected method %q", call.method)
		}
	}
	err := runSystemServiceJobsWithBus(context.Background(), bus, builtins.SystemServiceJobReload, []string{"api.service"})
	require.NoError(t, err)
	assert.Nil(t, bus.signalSink)
}

func TestRunSystemServiceJobsAcceptsSkippedResult(t *testing.T) {
	bus := &fakeManagerBus{}
	bus.respond = func(call fakeManagerCall) ([]any, error) {
		switch call.method {
		case systemdManagerIface + ".Subscribe":
			return nil, nil
		case systemdManagerIface + ".StartUnit":
			bus.signalSink <- &dbus.Signal{
				Path: systemdManagerPath,
				Name: systemdManagerIface + ".JobRemoved",
				Body: []any{uint32(78), dbus.ObjectPath(systemdJobPathPrefix + "78"), "api.service", "skipped"},
			}
			return []any{dbus.ObjectPath(systemdJobPathPrefix + "78")}, nil
		default:
			return nil, fmt.Errorf("unexpected method %q", call.method)
		}
	}
	require.NoError(t, runSystemServiceJobsWithBus(context.Background(), bus, builtins.SystemServiceJobStart, []string{"api.service"}))
}

func TestWaitSystemdJobClassifiesTerminalPaths(t *testing.T) {
	const selector = "api.service"
	expectedPath := dbus.ObjectPath(systemdJobPathPrefix + "42")
	jobRemoved := func(id uint32, path dbus.ObjectPath, unit, result string) *dbus.Signal {
		return &dbus.Signal{
			Path: systemdManagerPath,
			Name: systemdManagerIface + ".JobRemoved",
			Body: []any{id, path, unit, result},
		}
	}
	matching := func(result string) *dbus.Signal {
		return jobRemoved(42, expectedPath, selector, result)
	}

	unrelatedSignals := make([]*dbus.Signal, 0, maxManagerSignalsRead+1)
	for range maxManagerSignalsRead {
		unrelatedSignals = append(unrelatedSignals, jobRemoved(41, dbus.ObjectPath(systemdJobPathPrefix+"41"), "other.service", "done"))
	}
	unrelatedSignals = append(unrelatedSignals, matching("done"))

	tests := []struct {
		name          string
		signals       []*dbus.Signal
		closeSignals  bool
		cancelContext bool
		overflow      bool
		wantErr       string
		wantErrIs     error
		wantRemaining int
	}{
		{
			name:    "done",
			signals: []*dbus.Signal{matching("done")},
		},
		{
			name:    "skipped",
			signals: []*dbus.Signal{matching("skipped")},
		},
		{
			name:          "canceled context",
			cancelContext: true,
			wantErr:       "context canceled",
			wantErrIs:     context.Canceled,
		},
		{
			name:         "closed signal channel",
			closeSignals: true,
			wantErr:      `systemd manager connection closed while waiting for "api.service"`,
		},
		{
			name:    "nil signal",
			signals: []*dbus.Signal{nil},
			wantErr: "systemd manager returned a nil job signal",
		},
		{
			name: "malformed body",
			signals: []*dbus.Signal{{
				Path: systemdManagerPath,
				Name: systemdManagerIface + ".JobRemoved",
			}},
			wantErr: "systemd manager JobRemoved signal has an invalid body: dbus.Store: length mismatch",
		},
		{
			name:    "mismatched job ID and path",
			signals: []*dbus.Signal{jobRemoved(42, dbus.ObjectPath(systemdJobPathPrefix+"43"), selector, "done")},
			wantErr: "systemd manager returned an invalid job object path",
		},
		{
			name:    "invalid unit",
			signals: []*dbus.Signal{jobRemoved(42, expectedPath, "", "done")},
			wantErr: "systemd manager JobRemoved signal has an invalid unit: systemd unit name must not be empty",
		},
		{
			name:    "invalid result",
			signals: []*dbus.Signal{matching("")},
			wantErr: "job result must not be empty",
		},
		{
			name:    "failed result",
			signals: []*dbus.Signal{matching("failed")},
			wantErr: `systemd manager job for "api.service" finished with result "failed"`,
		},
		{
			name:    "canceled result",
			signals: []*dbus.Signal{matching("canceled")},
			wantErr: `systemd manager job for "api.service" finished with result "canceled"`,
		},
		{
			name:    "timeout result",
			signals: []*dbus.Signal{matching("timeout")},
			wantErr: `systemd manager job for "api.service" finished with result "timeout"`,
		},
		{
			name: "unrelated signals before matching result",
			signals: []*dbus.Signal{
				{Path: systemdManagerPath, Name: "org.example.Unrelated"},
				{Path: dbus.ObjectPath("/org/example"), Name: systemdManagerIface + ".JobRemoved"},
				jobRemoved(41, dbus.ObjectPath(systemdJobPathPrefix+"41"), "other.service", "done"),
				matching("done"),
			},
		},
		{
			name:     "signal queue overflow",
			overflow: true,
			wantErr:  `systemd manager signal queue overflowed while waiting for "api.service"`,
		},
		{
			name:          "unrelated signal limit",
			signals:       unrelatedSignals,
			wantErr:       `systemd manager emitted too many unrelated job results while waiting for "api.service"`,
			wantRemaining: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var ctx = context.Background()
			if test.cancelContext {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}

			signals := make(chan *dbus.Signal, len(test.signals))
			for _, signal := range test.signals {
				signals <- signal
			}
			if test.closeSignals {
				close(signals)
			}

			overflow := make(chan struct{})
			if test.overflow {
				close(overflow)
			}

			err := waitSystemdJob(ctx, signals, overflow, expectedPath, selector)
			require.Equal(t, test.wantRemaining, len(signals))
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, test.wantErr)
			if test.wantErrIs != nil {
				require.ErrorIs(t, err, test.wantErrIs)
			}
		})
	}
}

func TestRunSystemServiceJobsReportsPartialProgress(t *testing.T) {
	bus := &fakeManagerBus{}
	bus.respond = func(call fakeManagerCall) ([]any, error) {
		switch call.method {
		case systemdManagerIface + ".Subscribe":
			return nil, nil
		case systemdManagerIface + ".RestartUnit":
			unit := call.arguments[0].(string)
			if unit == "second.service" {
				return nil, errors.New("rejected")
			}
			bus.signalSink <- &dbus.Signal{
				Path: systemdManagerPath,
				Name: systemdManagerIface + ".JobRemoved",
				Body: []any{uint32(1), dbus.ObjectPath(systemdJobPathPrefix + "1"), unit, "done"},
			}
			return []any{dbus.ObjectPath(systemdJobPathPrefix + "1")}, nil
		default:
			return nil, fmt.Errorf("unexpected method %q", call.method)
		}
	}
	err := runSystemServiceJobsWithBus(context.Background(), bus, builtins.SystemServiceJobRestart, []string{"first.service", "second.service"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after completing 1 units (first.service)")
	assert.Contains(t, err.Error(), "not rolled back")
}

func TestRunSystemServiceJobsReportsAcceptedJobWithUnknownOutcome(t *testing.T) {
	bus := &fakeManagerBus{}
	bus.respond = func(call fakeManagerCall) ([]any, error) {
		switch call.method {
		case systemdManagerIface + ".Subscribe":
			return nil, nil
		case systemdManagerIface + ".StartUnit":
			return []any{"not-an-object-path"}, nil
		default:
			return nil, fmt.Errorf("unexpected method %q", call.method)
		}
	}
	err := runSystemServiceJobsWithBus(context.Background(), bus, builtins.SystemServiceJobStart, []string{"api.service"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `accepted a job for "api.service"`)
	assert.Contains(t, err.Error(), "final state is unknown")
	assert.Contains(t, err.Error(), "not rolled back")
}

func TestRunSystemServiceJobsDistinguishesTransportAmbiguityFromDBusRejection(t *testing.T) {
	tests := []struct {
		name          string
		callErr       error
		wantUncertain bool
	}{
		{name: "transport", callErr: context.DeadlineExceeded, wantUncertain: true},
		{name: "dbus rejection", callErr: dbus.Error{Name: "org.freedesktop.systemd1.UnitMasked"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bus := &fakeManagerBus{}
			bus.respond = func(call fakeManagerCall) ([]any, error) {
				if call.method == systemdManagerIface+".Subscribe" {
					return nil, nil
				}
				return nil, test.callErr
			}
			err := runSystemServiceJobsWithBus(context.Background(), bus, builtins.SystemServiceJobStart, []string{"api.service"})
			require.Error(t, err)
			assert.Equal(t, test.wantUncertain, strings.Contains(err.Error(), "may have accepted a job"))
		})
	}
}

func TestRunSystemServiceJobsReportsSignalOverflowAsUnknownOutcome(t *testing.T) {
	bus := &fakeManagerBus{overflow: make(chan struct{})}
	bus.respond = func(call fakeManagerCall) ([]any, error) {
		switch call.method {
		case systemdManagerIface + ".Subscribe":
			return nil, nil
		case systemdManagerIface + ".StartUnit":
			close(bus.overflow)
			return []any{dbus.ObjectPath(systemdJobPathPrefix + "88")}, nil
		default:
			return nil, fmt.Errorf("unexpected method %q", call.method)
		}
	}
	err := runSystemServiceJobsWithBus(context.Background(), bus, builtins.SystemServiceJobStart, []string{"api.service"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `accepted a job for "api.service"`)
	assert.Contains(t, err.Error(), "signal queue overflowed")
	assert.Contains(t, err.Error(), "final state is unknown")
}

func TestSystemServiceUnitFileMutations(t *testing.T) {
	units := []string{"api.service", "backup.timer"}
	type mutationTest struct {
		name            string
		method          string
		arguments       []any
		validChange     []any
		replyForChanges func([][]any) []any
		replyValues     int
		run             func(context.Context, managerBus, []string) error
	}
	mutations := []mutationTest{
		{
			name:        "enable",
			method:      "EnableUnitFiles",
			arguments:   []any{units, false, false},
			validChange: []any{"symlink", "/etc/systemd/system/multi-user.target.wants/api.service", "/usr/lib/systemd/system/api.service"},
			replyForChanges: func(changes [][]any) []any {
				return []any{true, changes}
			},
			replyValues: 2,
			run:         enableSystemServicesWithBus,
		},
		{
			name:        "disable",
			method:      "DisableUnitFiles",
			arguments:   []any{units, false},
			validChange: []any{"unlink", "/etc/systemd/system/multi-user.target.wants/api.service", ""},
			replyForChanges: func(changes [][]any) []any {
				return []any{changes}
			},
			replyValues: 1,
			run:         disableSystemServicesWithBus,
		},
	}

	const completedWarning = "unit-file changes completed, but manager reload/validation failed; changes were not rolled back: "
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			validChanges := [][]any{mutation.validChange}
			duplicateChanges := [][]any{mutation.validChange, mutation.validChange}
			tooManyChanges := make([][]any, maxUnitFileChanges+1)
			for index := range tooManyChanges {
				tooManyChanges[index] = mutation.validChange
			}
			methodFailureWarning := fmt.Sprintf("systemd manager %s failed; unit-file changes may have been partially applied and were not rolled back: ", mutation.method)
			tests := []struct {
				name                 string
				mutationReply        []any
				mutationErr          error
				reloadReply          []any
				reloadErr            error
				wantReload           bool
				wantErr              string
				wantErrIs            error
				wantCompletedWarning bool
			}{
				{
					name:          "success",
					mutationReply: mutation.replyForChanges(validChanges),
					wantReload:    true,
				},
				{
					name:        "D-Bus rejection",
					mutationErr: dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"},
					wantErr:     methodFailureWarning + fmt.Sprintf("systemd manager %s failed: org.freedesktop.DBus.Error.AccessDenied", mutation.method),
				},
				{
					name:        "transport ambiguity",
					mutationErr: context.DeadlineExceeded,
					wantErr:     methodFailureWarning + fmt.Sprintf("systemd manager %s failed: context deadline exceeded", mutation.method),
					wantErrIs:   context.DeadlineExceeded,
				},
				{
					name:                 "malformed mutation reply",
					wantErr:              completedWarning + fmt.Sprintf("systemd manager %s returned an invalid reply: reply has 0 values; expected %d", mutation.method, mutation.replyValues),
					wantCompletedWarning: true,
				},
				{
					name:                 "unsupported change record",
					mutationReply:        mutation.replyForChanges([][]any{{"copy", "/etc/systemd/system/api.service", "/usr/lib/systemd/system/api.service"}}),
					wantErr:              completedWarning + "systemd manager returned an unsupported unit-file change type",
					wantCompletedWarning: true,
				},
				{
					name:                 "duplicate change record",
					mutationReply:        mutation.replyForChanges(duplicateChanges),
					wantErr:              completedWarning + "systemd manager returned a duplicate unit-file change",
					wantCompletedWarning: true,
				},
				{
					name:                 "too many change records",
					mutationReply:        mutation.replyForChanges(tooManyChanges),
					wantErr:              completedWarning + fmt.Sprintf("systemd manager returned too many unit-file changes (maximum %d)", maxUnitFileChanges),
					wantCompletedWarning: true,
				},
				{
					name:                 "reload error",
					mutationReply:        mutation.replyForChanges(validChanges),
					reloadErr:            context.DeadlineExceeded,
					wantReload:           true,
					wantErr:              completedWarning + "systemd manager Reload failed: context deadline exceeded",
					wantErrIs:            context.DeadlineExceeded,
					wantCompletedWarning: true,
				},
				{
					name:                 "malformed reload reply",
					mutationReply:        mutation.replyForChanges(validChanges),
					reloadReply:          []any{true},
					wantReload:           true,
					wantErr:              completedWarning + "systemd manager Reload returned an invalid reply: reply has 1 values; expected 0",
					wantCompletedWarning: true,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					mutationMethod := systemdManagerIface + "." + mutation.method
					bus := &fakeManagerBus{respond: func(call fakeManagerCall) ([]any, error) {
						switch call.method {
						case mutationMethod:
							return test.mutationReply, test.mutationErr
						case systemdManagerIface + ".Reload":
							return test.reloadReply, test.reloadErr
						default:
							return nil, fmt.Errorf("unexpected method %q", call.method)
						}
					}}

					err := mutation.run(context.Background(), bus, units)
					if test.wantErr == "" {
						require.NoError(t, err)
					} else {
						require.EqualError(t, err, test.wantErr)
					}
					if test.wantErrIs != nil {
						require.ErrorIs(t, err, test.wantErrIs)
					}
					if test.wantCompletedWarning {
						require.ErrorContains(t, err, completedWarning)
					}

					wantCalls := []fakeManagerCall{{
						destination: systemdBusDestination,
						path:        systemdManagerPath,
						method:      mutationMethod,
						arguments:   mutation.arguments,
					}}
					if test.wantReload {
						wantCalls = append(wantCalls, fakeManagerCall{
							destination: systemdBusDestination,
							path:        systemdManagerPath,
							method:      systemdManagerIface + ".Reload",
						})
					}
					require.Equal(t, wantCalls, bus.calls)
				})
			}
		})
	}
}

func TestManagerBoundaryValidation(t *testing.T) {
	for _, unit := range []string{"api.service", "backup.timer", "events.socket", "-.mount", "templ@.service"} {
		require.NoError(t, validateManagerUnit(unit), unit)
	}
	for _, unit := range []string{"api", `real\x2dname.service`, "tenant:api.service", "api.snapshot", "two@@x.service"} {
		require.Error(t, validateManagerUnit(unit), unit)
	}
	for _, unit := range []string{`real\x2dname.service`, "tenant:api.service", "templ@instance.service"} {
		require.NoError(t, validateManagerCanonicalUnit(unit), unit)
	}
	require.Error(t, validateManagerCanonicalUnit(`bad\q00.service`))
	require.Error(t, validateReturnedManagerJobPath(dbus.ObjectPath(systemdJobPathPrefix+"not-a-number")))
	require.NoError(t, validateReturnedManagerJobPath(dbus.ObjectPath(systemdJobPathPrefix+"123")))
	require.Error(t, validateManagerObjectPath("unit", dbus.ObjectPath("/attacker/unit/1"), systemdUnitPathPrefix))
	require.NoError(t, validateManagerObjectPath("unit", dbus.ObjectPath(systemdUnitPathPrefix+"api_2eservice"), systemdUnitPathPrefix))
	require.Error(t, validateManagerObjectPath("unit", dbus.ObjectPath(systemdUnitPathPrefix+"api_2eservice/extra"), systemdUnitPathPrefix))

	tooMany := make([]string, builtins.MaxSystemServiceOperands+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("unit-%d.service", index)
	}
	require.Error(t, validateManagerUnits(tooMany, false))
	require.Error(t, validateManagerUnits([]string{"api.service", "api.service"}, false))
	require.Error(t, validateManagerString("description", strings.Repeat("x", builtins.MaxSystemServiceFieldBytes+1), false))
}
