// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemctl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

type fakeStateReader struct {
	listRequest  builtins.SystemServiceListRequest
	listStates   []builtins.SystemServiceState
	listErr      error
	listCalls    int
	inspectUnits []string
	inspectState []builtins.SystemServiceState
	inspectErr   error
	inspectCalls int
}

func (f *fakeStateReader) ListSystemServices(_ context.Context, request builtins.SystemServiceListRequest) ([]builtins.SystemServiceState, error) {
	f.listCalls++
	f.listRequest = builtins.SystemServiceListRequest{
		Services:        append([]string(nil), request.Services...),
		IncludeInactive: request.IncludeInactive,
	}
	return append([]builtins.SystemServiceState(nil), f.listStates...), f.listErr
}

func (f *fakeStateReader) InspectSystemServices(_ context.Context, units []string) ([]builtins.SystemServiceState, error) {
	f.inspectCalls++
	f.inspectUnits = append([]string(nil), units...)
	return append([]builtins.SystemServiceState(nil), f.inspectState...), f.inspectErr
}

type jobCall struct {
	action builtins.SystemServiceJobAction
	units  []string
}

type fakeController struct {
	jobs         []jobCall
	jobErr       error
	enableUnits  []string
	enableErr    error
	disableUnits []string
	disableErr   error
}

func (f *fakeController) RunSystemServiceJobs(_ context.Context, action builtins.SystemServiceJobAction, units []string) error {
	f.jobs = append(f.jobs, jobCall{action: action, units: append([]string(nil), units...)})
	return f.jobErr
}

func (f *fakeController) EnableSystemServices(_ context.Context, units []string) error {
	f.enableUnits = append([]string(nil), units...)
	return f.enableErr
}

func (f *fakeController) DisableSystemServices(_ context.Context, units []string) error {
	f.disableUnits = append([]string(nil), units...)
	return f.disableErr
}

type invocation struct {
	result     builtins.Result
	stdout     string
	stderr     string
	authorized []builtins.SystemdOperation
}

func runSystemctl(t *testing.T, args []string, callCtx *builtins.CallContext) invocation {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if callCtx.Stdout == nil {
		callCtx.Stdout = &stdout
	}
	if callCtx.Stderr == nil {
		callCtx.Stderr = &stderr
	}

	fs := pflag.NewFlagSet("systemctl", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	handler := makeFlags(fs)
	require.NoError(t, fs.Parse(args))
	result := handler(context.Background(), callCtx, fs.Args())
	return invocation{result: result, stdout: stdout.String(), stderr: stderr.String()}
}

func permissiveContext(reader builtins.SystemServiceStateReader, controller builtins.SystemServiceController, authorized *[]builtins.SystemdOperation) *builtins.CallContext {
	return &builtins.CallContext{
		RemediationMode: true,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			if authorized != nil {
				*authorized = append(*authorized, operations...)
			}
			return nil
		},
		Systemd: &builtins.SystemdServices{ServiceState: reader, ServiceControl: controller},
	}
}

func TestSystemctlRequiresRemediationModeBeforeAnyCapability(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "bare list"},
		{name: "explicit list", args: []string{"list-units"}},
		{name: "help", args: []string{"--help"}},
		{name: "status", args: []string{"status", "api.service"}},
		{name: "start", args: []string{"start", "api.service"}},
		{name: "stop", args: []string{"stop", "api.service"}},
		{name: "reload", args: []string{"reload", "api.service"}},
		{name: "restart", args: []string{"restart", "api.service"}},
		{name: "enable", args: []string{"enable", "api.service"}},
		{name: "disable", args: []string{"disable", "api.service"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &fakeStateReader{}
			controller := &fakeController{}
			capabilityCalls := 0
			callCtx := &builtins.CallContext{
				ReadableSystemServices: func() []string {
					capabilityCalls++
					return []string{"api.service"}
				},
				AuthorizeSystemd: func(...builtins.SystemdOperation) error {
					capabilityCalls++
					return nil
				},
				Systemd: &builtins.SystemdServices{ServiceState: reader, ServiceControl: controller},
			}

			got := runSystemctl(t, test.args, callCtx)

			assert.Equal(t, uint8(1), got.result.Code)
			assert.Empty(t, got.stdout)
			assert.Equal(t, "systemctl: remediation mode required\n", got.stderr)
			assert.Zero(t, capabilityCalls)
			assert.Zero(t, reader.listCalls)
			assert.Zero(t, reader.inspectCalls)
			assert.Empty(t, controller.jobs)
			assert.Empty(t, controller.enableUnits)
			assert.Empty(t, controller.disableUnits)
		})
	}
	assert.True(t, Cmd.RemediationOnly)
}

func TestRemovedCommandsAreRejectedBeforeCapabilities(t *testing.T) {
	for _, verb := range []string{
		"show",
		"is-active",
		"is-failed",
		"is-enabled",
		"try-restart",
		"reload-or-restart",
		"try-reload-or-restart",
		"reset-failed",
	} {
		t.Run(verb, func(t *testing.T) {
			reader := &fakeStateReader{}
			controller := &fakeController{}
			capabilityCalls := 0
			callCtx := permissiveContext(reader, controller, nil)
			callCtx.ReadableSystemServices = func() []string {
				capabilityCalls++
				return []string{"api.service"}
			}
			callCtx.AuthorizeSystemd = func(...builtins.SystemdOperation) error {
				capabilityCalls++
				return nil
			}

			got := runSystemctl(t, []string{verb, "api.service"}, callCtx)

			assert.Equal(t, uint8(1), got.result.Code)
			assert.Empty(t, got.stdout)
			assert.Equal(t, "systemctl: unsupported command \""+verb+"\"\nTry 'systemctl --help' for more information.\n", got.stderr)
			assert.Zero(t, capabilityCalls)
			assert.Zero(t, reader.listCalls)
			assert.Zero(t, reader.inspectCalls)
			assert.Empty(t, controller.jobs)
			assert.Empty(t, controller.enableUnits)
			assert.Empty(t, controller.disableUnits)
		})
	}
}

func unitState(name, active, sub string) builtins.SystemServiceState {
	return builtins.SystemServiceState{
		Name:          name,
		Description:   name + " description",
		LoadState:     "loaded",
		ActiveState:   active,
		SubState:      sub,
		UnitFileState: "enabled",
		Result:        "success",
	}
}

func TestListUnitsUsesOnlyFilteredReadableGrants(t *testing.T) {
	reader := &fakeStateReader{listStates: []builtins.SystemServiceState{
		unitState("worker.socket", "inactive", "dead"),
		unitState("api.service", "active", "running"),
	}}
	var authorized []builtins.SystemdOperation
	callCtx := permissiveContext(reader, nil, &authorized)
	callCtx.ReadableSystemServices = func() []string {
		return []string{"legacy", "db.timer", "worker.socket", "api.service"}
	}

	got := runSystemctl(t, []string{"list-units", "--all", "--type=service,socket", "--state=active"}, callCtx)

	assert.Equal(t, uint8(0), got.result.Code)
	assert.Empty(t, got.stderr)
	assert.Equal(t, "UNIT LOAD ACTIVE SUB DESCRIPTION\napi.service loaded active running api.service description\n1 units listed (restricted to units granted read access).\n", got.stdout)
	assert.Equal(t, builtins.SystemServiceListRequest{
		Services:        []string{"api.service", "worker.socket"},
		IncludeInactive: true,
	}, reader.listRequest)
	assert.Equal(t, []builtins.SystemdOperation{
		{Service: "api.service", Action: builtins.SystemServiceRead},
		{Service: "worker.socket", Action: builtins.SystemServiceRead},
	}, authorized)
}

func TestBareListPrefiltersTypeBeforeOperandCap(t *testing.T) {
	readable := []string{"only.service", "legacy"}
	for i := 0; i <= builtins.MaxSystemServiceOperands; i++ {
		readable = append(readable, strings.Repeat("t", i+1)+".timer")
	}
	reader := &fakeStateReader{listStates: []builtins.SystemServiceState{unitState("only.service", "active", "running")}}
	callCtx := permissiveContext(reader, nil, nil)
	callCtx.ReadableSystemServices = func() []string { return readable }

	got := runSystemctl(t, []string{"--type=service", "--no-legend", "--system", "--no-pager"}, callCtx)

	assert.Equal(t, uint8(0), got.result.Code)
	assert.Equal(t, "only.service loaded active running only.service description\n", got.stdout)
	assert.Empty(t, got.stderr)
	assert.Equal(t, []string{"only.service"}, reader.listRequest.Services)
}

func TestListUnitsAuthorizesCompleteSetBeforeBackend(t *testing.T) {
	reader := &fakeStateReader{}
	var calls int
	callCtx := permissiveContext(reader, nil, nil)
	callCtx.ReadableSystemServices = func() []string { return []string{"a.service", "b.timer"} }
	callCtx.AuthorizeSystemd = func(operations ...builtins.SystemdOperation) error {
		calls++
		assert.Len(t, operations, 2)
		return errors.New("denied\n\x1b[31m")
	}

	got := runSystemctl(t, nil, callCtx)

	assert.Equal(t, uint8(1), got.result.Code)
	assert.Equal(t, 1, calls)
	assert.Zero(t, reader.listCalls)
	assert.NotContains(t, got.stderr, "\n\x1b")
	assert.NotContains(t, got.stderr, "\x1b")
	assert.Equal(t, "systemctl: denied??[31m\n", got.stderr)
}

func TestListUnitsRejectsBackendUnitOutsideReadableSet(t *testing.T) {
	reader := &fakeStateReader{listStates: []builtins.SystemServiceState{unitState("secret.service", "active", "running")}}
	callCtx := permissiveContext(reader, nil, nil)
	callCtx.ReadableSystemServices = func() []string { return []string{"api.service"} }

	got := runSystemctl(t, nil, callCtx)

	assert.Equal(t, uint8(1), got.result.Code)
	assert.Empty(t, got.stdout)
	assert.Contains(t, got.stderr, "unauthorized unit")
}

func TestExplicitOperandsAreValidatedBeforeAuthorization(t *testing.T) {
	tests := []string{
		"legacy",
		"bad.unit",
		"*.service",
		"tenant:api.service",
		"path/api.service",
		`escaped\x2dapi.service`,
		"café.service",
		"@instance.service",
		"a@b@c.service",
		strings.Repeat("a", builtins.MaxSystemServiceNameBytes) + ".service",
	}
	for _, unit := range tests {
		t.Run(strings.ReplaceAll(unit, "/", "_"), func(t *testing.T) {
			var authorizeCalls int
			reader := &fakeStateReader{}
			callCtx := permissiveContext(reader, nil, nil)
			callCtx.AuthorizeSystemd = func(...builtins.SystemdOperation) error {
				authorizeCalls++
				return nil
			}

			got := runSystemctl(t, []string{"status", unit}, callCtx)

			assert.Equal(t, uint8(1), got.result.Code)
			assert.Zero(t, authorizeCalls)
			assert.Zero(t, reader.inspectCalls)
			assert.NotEmpty(t, got.stderr)
		})
	}
}

func TestArgumentTokenCountsAreBoundedBeforeBackendWork(t *testing.T) {
	duplicateUnits := make([]string, builtins.MaxSystemServiceOperands+1)
	for index := range duplicateUnits {
		duplicateUnits[index] = "api.service"
	}
	_, err := validateUnits(duplicateUnits, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many unit operands")

	_, err = parseFilterValues([]string{strings.Repeat("active,", 100_000)}, "state", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many --state values")

	tooLong := strings.Repeat("x", maxFilterValueBytes+1)
	_, err = parseFilterValues([]string{tooLong}, "state", false)
	require.EqualError(t, err, "--state value exceeds 64 bytes")
	assert.NotContains(t, err.Error(), tooLong)
}

func TestStatusDeduplicatesAndFormatsBoundedState(t *testing.T) {
	state := unitState("api.service", "active", "running")
	state.Description = "API"
	state.MainPID = 42
	reader := &fakeStateReader{inspectState: []builtins.SystemServiceState{state}}
	var authorized []builtins.SystemdOperation

	got := runSystemctl(t, []string{"status", "api.service", "api.service"}, permissiveContext(reader, nil, &authorized))

	assert.Equal(t, uint8(0), got.result.Code)
	assert.Empty(t, got.stderr)
	assert.Equal(t, "api.service - API\n     Loaded: loaded (enabled)\n     Active: active (running)\n   Main PID: 42\n     Result: success\n", got.stdout)
	assert.Equal(t, []string{"api.service"}, reader.inspectUnits)
	assert.Equal(t, []builtins.SystemdOperation{{Service: "api.service", Action: builtins.SystemServiceRead}}, authorized)
}

func TestStatusExitCodes(t *testing.T) {
	tests := []struct {
		name  string
		state builtins.SystemServiceState
		code  uint8
	}{
		{name: "inactive", state: unitState("api.service", "inactive", "dead"), code: 3},
		{name: "missing", state: builtins.SystemServiceState{Name: "api.service", LoadState: "not-found", ActiveState: "inactive", SubState: "dead"}, code: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &fakeStateReader{inspectState: []builtins.SystemServiceState{test.state}}
			got := runSystemctl(t, []string{"status", "api.service"}, permissiveContext(reader, nil, nil))
			assert.Equal(t, test.code, got.result.Code)
		})
	}
}

func TestBackendStringsCannotInjectTerminalControls(t *testing.T) {
	state := unitState("api.service", "active", "running")
	state.Description = "hello\nworld\x1b\t\x00\u202e\xff"
	reader := &fakeStateReader{inspectState: []builtins.SystemServiceState{state}}

	got := runSystemctl(t, []string{"status", "api.service"}, permissiveContext(reader, nil, nil))

	assert.Equal(t, uint8(0), got.result.Code)
	assert.Equal(t, "api.service - hello?world?????\n     Loaded: loaded (enabled)\n     Active: active (running)\n     Result: success\n", got.stdout)
	assert.NotContains(t, got.stdout, "\x1b")
	assert.Empty(t, got.stderr)

	state.Description = "safe"
	state.ActiveState = "active\nforged"
	reader.inspectState = []builtins.SystemServiceState{state}
	got = runSystemctl(t, []string{"status", "api.service"}, permissiveContext(reader, nil, nil))
	assert.Equal(t, uint8(1), got.result.Code)
	assert.Empty(t, got.stdout)
	assert.NotContains(t, got.stderr, "\nforged")
	assert.NotContains(t, got.stderr, "\x1b")
}

func TestJobVerbsUseFixedBackendActionsAndAuthorizations(t *testing.T) {
	tests := []struct {
		verb    string
		job     builtins.SystemServiceJobAction
		actions []builtins.SystemServiceAction
	}{
		{verb: "start", job: builtins.SystemServiceJobStart, actions: []builtins.SystemServiceAction{builtins.SystemServiceStart}},
		{verb: "stop", job: builtins.SystemServiceJobStop, actions: []builtins.SystemServiceAction{builtins.SystemServiceStop}},
		{verb: "reload", job: builtins.SystemServiceJobReload, actions: []builtins.SystemServiceAction{builtins.SystemServiceReload}},
		{verb: "restart", job: builtins.SystemServiceJobRestart, actions: []builtins.SystemServiceAction{builtins.SystemServiceRestart}},
	}
	for _, test := range tests {
		t.Run(test.verb, func(t *testing.T) {
			controller := &fakeController{}
			var authorized []builtins.SystemdOperation
			got := runSystemctl(t, []string{test.verb, "api.service", "api.service"}, permissiveContext(nil, controller, &authorized))
			assert.Equal(t, uint8(0), got.result.Code)
			require.Len(t, controller.jobs, 1)
			assert.Equal(t, test.job, controller.jobs[0].action)
			assert.Equal(t, []string{"api.service"}, controller.jobs[0].units)
			expected := make([]builtins.SystemdOperation, 0, len(test.actions))
			for _, action := range test.actions {
				expected = append(expected, builtins.SystemdOperation{Service: "api.service", Action: action})
			}
			assert.Equal(t, expected, authorized)
		})
	}
}

func TestEnableDisableUseDedicatedBackendAndAuthorization(t *testing.T) {
	tests := []struct {
		verb       string
		action     builtins.SystemServiceAction
		configured func(*fakeController) []string
	}{
		{verb: "enable", action: builtins.SystemServiceEnable, configured: func(controller *fakeController) []string { return controller.enableUnits }},
		{verb: "disable", action: builtins.SystemServiceDisable, configured: func(controller *fakeController) []string { return controller.disableUnits }},
	}
	for _, test := range tests {
		t.Run(test.verb, func(t *testing.T) {
			controller := &fakeController{}
			var authorized []builtins.SystemdOperation

			got := runSystemctl(t, []string{test.verb, "api.service", "api.service"}, permissiveContext(nil, controller, &authorized))

			assert.Equal(t, uint8(0), got.result.Code)
			assert.Empty(t, got.stdout)
			assert.Empty(t, got.stderr)
			assert.Equal(t, []string{"api.service"}, test.configured(controller))
			assert.Equal(t, []builtins.SystemdOperation{{Service: "api.service", Action: test.action}}, authorized)
			assert.Empty(t, controller.jobs)
		})
	}
}

func TestCanceledMutationStillReportsUncertainOutcome(t *testing.T) {
	controller := &fakeController{jobErr: errors.New("job accepted, final state is unknown and was not rolled back")}
	callCtx := permissiveContext(nil, controller, nil)
	var stdout, stderr bytes.Buffer
	callCtx.Stdout = &stdout
	callCtx.Stderr = &stderr

	fs := pflag.NewFlagSet("systemctl", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	handler := makeFlags(fs)
	require.NoError(t, fs.Parse([]string{"restart", "api.service"}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := handler(ctx, callCtx, fs.Args())
	assert.Equal(t, uint8(1), result.Code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "job accepted")
	assert.Contains(t, stderr.String(), "not rolled back")
}

func TestFlagsAreScopedToTheirCommands(t *testing.T) {
	tests := [][]string{
		{"status", "api.service", "--all"},
		{"status", "api.service", "--state=active"},
		{"start", "api.service", "--type=service"},
		{"enable", "api.service", "--no-legend"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			got := runSystemctl(t, args, permissiveContext(&fakeStateReader{}, &fakeController{}, nil))
			assert.Equal(t, uint8(1), got.result.Code)
			assert.Contains(t, got.stderr, "is not supported with")
		})
	}
}

func TestDangerousHostSystemctlOptionsAreRejected(t *testing.T) {
	Cmd.Register()
	handler, ok := builtins.Lookup("systemctl")
	require.True(t, ok)

	for _, args := range [][]string{
		{"--root=/host", "status", "api.service"},
		{"--machine=host", "status", "api.service"},
		{"--user", "status", "api.service"},
		{"--global", "enable", "api.service"},
		{"--runtime", "enable", "api.service"},
		{"--force", "restart", "api.service"},
		{"--job-mode=ignore-dependencies", "restart", "api.service"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			result := handler(context.Background(), &builtins.CallContext{Stdout: &stdout, Stderr: &stderr, RemediationMode: true}, args)
			assert.Equal(t, uint8(1), result.Code)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "unrecognized option")
		})
	}

	for _, test := range []struct {
		option  string
		args    []string
		wantErr string
	}{
		{option: "--now", args: []string{"enable", "api.service", "--now"}, wantErr: "unrecognized option '--now'"},
		{option: "--property", args: []string{"status", "api.service", "--property"}, wantErr: "unrecognized option '--property'"},
		{option: "-p", args: []string{"status", "api.service", "-p"}, wantErr: "invalid option -- 'p'"},
		{option: "--value", args: []string{"status", "api.service", "--value"}, wantErr: "unrecognized option '--value'"},
		{option: "--quiet", args: []string{"status", "api.service", "--quiet"}, wantErr: "unrecognized option '--quiet'"},
		{option: "-q", args: []string{"status", "api.service", "-q"}, wantErr: "invalid option -- 'q'"},
	} {
		t.Run("removed_"+strings.TrimLeft(test.option, "-"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			result := handler(context.Background(), &builtins.CallContext{Stdout: &stdout, Stderr: &stderr, RemediationMode: true}, test.args)
			assert.Equal(t, uint8(1), result.Code)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "systemctl: "+test.wantErr+"\nTry 'systemctl --help' for more information.\n", stderr.String())
		})
	}
}

func TestHelpDocumentsRestrictedEnumerationWithoutCapabilities(t *testing.T) {
	got := runSystemctl(t, []string{"--help"}, &builtins.CallContext{RemediationMode: true})

	assert.Equal(t, uint8(0), got.result.Code)
	assert.Empty(t, got.stderr)
	assert.Contains(t, got.stdout, "Usage: systemctl")
	assert.Contains(t, got.stdout, "entire command is available only in remediation mode")
	assert.Contains(t, got.stdout, "exact units granted read access")
	assert.Contains(t, got.stdout, "--system")
	assert.Contains(t, got.stdout, "--no-pager")
	assert.Contains(t, got.stdout, "--type")
	for _, retained := range []string{
		"  list-units                 List read-authorized units",
		"  status UNIT...             Show bounded unit status without logs",
		"  start|stop|reload UNIT...  Queue and wait for an authorized job",
		"  restart UNIT...            Queue and wait for an authorized job",
		"  enable|disable UNIT...     Change unit-file state",
	} {
		assert.Contains(t, got.stdout, retained)
	}
	for _, removed := range []string{
		"  show UNIT",
		"is-active",
		"is-failed",
		"is-enabled",
		"try-restart",
		"reload-or-restart",
		"try-reload-or-restart",
		"reset-failed",
		"--now",
		"--property",
		"--value",
		"--quiet",
	} {
		assert.NotContains(t, got.stdout, removed)
	}
}
