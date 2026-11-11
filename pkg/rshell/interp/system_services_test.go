// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func TestAllowedSystemServicesAuthorizesExactServiceAndAction(t *testing.T) {
	runner, err := New(
		WithMode(ModeRemediation),
		AllowedSystemServices([]SystemServiceControlGrant{
			{
				Service: "mysql.service",
				Actions: []SystemServiceAction{
					SystemServiceRead,
					SystemServiceClean,
					SystemServiceStart,
					SystemServiceStop,
					SystemServiceReload,
					SystemServiceRestart,
					SystemServiceEnable,
					SystemServiceDisable,
				},
			},
			{
				Service: "nginx.service",
				Actions: []SystemServiceAction{SystemServiceRead},
			},
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	for _, action := range []SystemServiceAction{
		SystemServiceRead,
		SystemServiceClean,
		SystemServiceStart,
		SystemServiceStop,
		SystemServiceReload,
		SystemServiceRestart,
		SystemServiceEnable,
		SystemServiceDisable,
	} {
		require.NoError(t, runner.authorizeSystemServices(action, "mysql.service"), "action %q", action)
	}
	require.NoError(t, runner.authorizeSystemServices(SystemServiceRead, "mysql.service", "nginx.service"))

	err = runner.authorizeSystemServices(SystemServiceRestart, "nginx.service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `system service "nginx.service" is not allowed for action "restart"`)

	for _, service := range []string{"mysql", "MYSQL.service"} {
		err = runner.authorizeSystemServices(SystemServiceRead, service)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `system service "`+service+`" is not allowed`)
	}
}

func TestAllowedSystemServicesWildcardExpandsToAllSupportedActions(t *testing.T) {
	runner, err := New(
		WithMode(ModeRemediation),
		AllowedSystemServices([]SystemServiceControlGrant{
			{Service: "mysql.service", Actions: []SystemServiceAction{SystemServiceAllActions}},
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	operations := runner.allowedSystemServicesList()
	require.Len(t, operations, len(systemServiceActionOrder))
	for i, action := range systemServiceActionOrder {
		assert.Equal(t, SystemdOperation{Service: "mysql.service", Action: action}, operations[i])
		require.NoError(t, runner.authorizeSystemServices(action, "mysql.service"), "action %q", action)
	}
	assert.Equal(t, []string{"mysql.service"}, runner.readableSystemServices())
	require.Error(t, runner.authorizeSystemServices(SystemServiceRead, "postgres.service"))
}

func TestAllowedSystemServicesDefaultDenyIsIndependentOfAllowedCommands(t *testing.T) {
	runner, err := New(WithMode(ModeRemediation), allowAllCommandsOpt())
	require.NoError(t, err)
	defer runner.Close()

	err = runner.authorizeSystemServices(SystemServiceRead, "mysql.service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestAllowedSystemServicesKeepsSharedJournalReadOutsideRemediationMode(t *testing.T) {
	// The shared read action remains available to journalctl. The systemctl
	// builtin applies its separate command-wide remediation-mode gate.
	runner, err := New(AllowedSystemServices([]SystemServiceControlGrant{
		{
			Service: "mysql.service",
			Actions: []SystemServiceAction{SystemServiceAllActions},
		},
	}))
	require.NoError(t, err)
	defer runner.Close()

	require.NoError(t, runner.authorizeSystemServices(SystemServiceRead, "mysql.service"))
	for _, action := range []SystemServiceAction{
		SystemServiceClean,
		SystemServiceStart,
		SystemServiceStop,
		SystemServiceReload,
		SystemServiceRestart,
		SystemServiceEnable,
		SystemServiceDisable,
	} {
		err = runner.authorizeSystemServices(action, "mysql.service")
		require.Error(t, err, "action %q", action)
		assert.Contains(t, err.Error(), `action "`+string(action)+`" requires remediation mode`)
	}
}

func TestAllowedSystemServicesReadableExactGrants(t *testing.T) {
	runner, err := New(
		WithMode(ModeRemediation),
		AllowedSystemServices([]SystemServiceControlGrant{
			{Service: "nightly.timer", Actions: []SystemServiceAction{SystemServiceRead, SystemServiceStart}},
			{Service: "api.socket", Actions: []SystemServiceAction{SystemServiceRead, SystemServiceStop}},
			{Service: "worker.service", Actions: []SystemServiceAction{SystemServiceRestart}},
			{Service: "dbus.service", Actions: []SystemServiceAction{SystemServiceRead}},
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	want := []string{"api.socket", "dbus.service", "nightly.timer"}
	readable := runner.readableSystemServices()
	assert.Equal(t, want, readable)

	// The configured exact names remain valid regardless of unit type. A
	// similarly named service unit is not implicitly granted by a timer grant.
	require.NoError(t, runner.authorizeSystemServices(SystemServiceRead, "nightly.timer", "api.socket"))
	require.NoError(t, runner.authorizeSystemServices(SystemServiceStart, "nightly.timer"))
	require.NoError(t, runner.authorizeSystemServices(SystemServiceStop, "api.socket"))
	err = runner.authorizeSystemServices(SystemServiceRead, "nightly.service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `system service "nightly.service" is not allowed for action "read"`)

	// Callers cannot mutate the runner's policy through the returned slice.
	readable[0] = "changed.service"
	assert.Equal(t, want, runner.readableSystemServices())
}

func TestAllowedSystemServicesListIsCanonicalAndDefensive(t *testing.T) {
	runner, err := New(
		AllowedSystemServices([]SystemServiceControlGrant{
			{
				Service: "worker.service",
				Actions: []SystemServiceAction{
					SystemServiceEnable,
					SystemServiceClean,
					SystemServiceRestart,
				},
			},
			{
				Service: "api.socket",
				Actions: []SystemServiceAction{
					SystemServiceStop,
					SystemServiceRead,
					SystemServiceStop,
				},
			},
			{
				Service: "nightly.timer",
				Actions: []SystemServiceAction{SystemServiceStart},
			},
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	want := []SystemdOperation{
		{Service: "api.socket", Action: SystemServiceRead},
		{Service: "api.socket", Action: SystemServiceStop},
		{Service: "nightly.timer", Action: SystemServiceStart},
		{Service: "worker.service", Action: SystemServiceClean},
		{Service: "worker.service", Action: SystemServiceRestart},
		{Service: "worker.service", Action: SystemServiceEnable},
	}
	operations := runner.allowedSystemServicesList()
	assert.Equal(t, want, operations)

	operations[0].Service = "changed.service"
	assert.Equal(t, want, runner.allowedSystemServicesList())
}

func TestAllowedSystemServicesReadDoesNotEnableMutation(t *testing.T) {
	runner, err := New(
		WithMode(ModeRemediation),
		AllowedSystemServices([]SystemdControlGrant{
			{Service: "systemd-journald.service", Actions: []SystemServiceAction{SystemServiceRead}},
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	require.NoError(t, runner.authorizeSystemd(
		SystemdOperation{Service: "systemd-journald.service", Action: SystemServiceRead},
	))
	for _, action := range []SystemServiceAction{
		SystemServiceClean,
		SystemServiceStart,
		SystemServiceStop,
		SystemServiceReload,
		SystemServiceRestart,
		SystemServiceEnable,
		SystemServiceDisable,
	} {
		err = runner.authorizeSystemd(
			SystemdOperation{Service: "systemd-journald.service", Action: action},
		)
		require.Error(t, err, "action %q", action)
		assert.Contains(t, err.Error(), `is not allowed for action "`+string(action)+`"`)
	}
}

func TestAllowedSystemServicesCopiesAndCombinesGrants(t *testing.T) {
	actions := []SystemServiceAction{SystemServiceRead}
	grants := []SystemServiceControlGrant{
		{Service: "mysql", Actions: actions},
		{Service: "mysql", Actions: []SystemServiceAction{SystemServiceRestart}},
	}
	runner, err := New(WithMode(ModeRemediation), AllowedSystemServices(grants))
	require.NoError(t, err)
	defer runner.Close()

	grants[0].Service = "changed"
	actions[0] = SystemServiceReload

	require.NoError(t, runner.authorizeSystemServices(SystemServiceRead, "mysql"))
	require.NoError(t, runner.authorizeSystemServices(SystemServiceRestart, "mysql"))
}

func TestAllowedSystemServicesSkipsEmptyAndInvalidGrants(t *testing.T) {
	var warningOutput bytes.Buffer
	missingPath := filepath.Join(t.TempDir(), "missing")
	runner, err := New(
		WithMode(ModeRemediation),
		WarningsWriter(&warningOutput),
		AllowedSystemServices([]SystemServiceControlGrant{
			{Service: "ignored.service"},
			{Service: "mysql.service", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "mysql service", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "mysql\u00a0service", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "/etc/systemd/system/mysql.service", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "C:\\svc", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "..\\svc", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "tenant:mysql.service", Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: "mysql*.service", Actions: []SystemServiceAction{SystemServiceRead}},
		}),
		// Applying AllowedPaths after AllowedSystemServices verifies that one
		// allowlist option does not overwrite another option's warnings.
		AllowedPaths([]string{missingPath}),
	)
	require.NoError(t, err)
	defer runner.Close()

	require.NoError(t, runner.authorizeSystemServices(SystemServiceRead, "mysql.service"))
	assert.Len(t, runner.allowedSystemServices, 1)
	assert.NotContains(t, runner.allowedSystemServices, "ignored.service")

	warnings := runner.Warnings()
	require.Len(t, warnings, 9)
	for _, needle := range []string{
		"AllowedSystemServices: skipping grant 2: system service name must not be empty",
		"whitespace or control characters",
		`AllowedSystemServices: skipping grant 6: system service name "C:\\svc" must not contain a path separator`,
		`AllowedSystemServices: skipping grant 7: system service name "..\\svc" must not contain a path separator`,
		"path separator",
		"must not contain ':'",
		"glob pattern",
		"AllowedPaths: skipping",
	} {
		assert.Contains(t, warningOutput.String(), needle)
	}
	assert.NotContains(t, warningOutput.String(), "ignored.service")
}

func TestAllowedSystemServicesValidatesNameEncodingAndLength(t *testing.T) {
	maxUnit := strings.Repeat("a", builtins.MaxSystemServiceNameBytes-len(".service")) + ".service"
	tooLongUnit := "x" + maxUnit
	invalidUTF8Unit := string([]byte{'a', 'p', 'i', 0xff, '.', 's', 'e', 'r', 'v', 'i', 'c', 'e'})

	var warningOutput bytes.Buffer
	runner, err := New(
		WarningsWriter(&warningOutput),
		AllowedSystemServices([]SystemServiceControlGrant{
			{Service: maxUnit, Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: tooLongUnit, Actions: []SystemServiceAction{SystemServiceRead}},
			{Service: invalidUTF8Unit, Actions: []SystemServiceAction{SystemServiceRead}},
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	require.NoError(t, runner.authorizeSystemServices(SystemServiceRead, maxUnit))
	assert.Len(t, runner.allowedSystemServices, 1)
	require.Len(t, runner.Warnings(), 2)
	assert.Contains(t, warningOutput.String(), "system service name exceeds 255 bytes")
	assert.Contains(t, warningOutput.String(), "system service name must be valid UTF-8")
}

func TestAllowedSystemServicesSkipsUnsupportedActions(t *testing.T) {
	var warningOutput bytes.Buffer
	runner, err := New(
		WithMode(ModeRemediation),
		WarningsWriter(&warningOutput),
		AllowedSystemServices([]SystemServiceControlGrant{
			{Service: "mysql.service", Actions: []SystemServiceAction{SystemServiceRead, SystemServiceStop, "freeze", SystemServiceAction("reset-failed"), SystemServiceReload}},
			{Service: "ignored.service", Actions: []SystemServiceAction{"mask"}},
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	require.NoError(t, runner.authorizeSystemServices(SystemServiceRead, "mysql.service"))
	require.NoError(t, runner.authorizeSystemServices(SystemServiceStop, "mysql.service"))
	require.NoError(t, runner.authorizeSystemServices(SystemServiceReload, "mysql.service"))
	assert.NotContains(t, runner.allowedSystemServices, "ignored.service")

	warnings := runner.Warnings()
	require.Len(t, warnings, 3)
	assert.Contains(t, warningOutput.String(), `AllowedSystemServices: skipping unsupported action "freeze" in grant 0 for "mysql.service"`)
	assert.Contains(t, warningOutput.String(), `AllowedSystemServices: skipping unsupported action "reset-failed" in grant 0 for "mysql.service"`)
	assert.Contains(t, warningOutput.String(), `AllowedSystemServices: skipping unsupported action "mask" in grant 1 for "ignored.service"`)
}

func TestAuthorizeSystemServicesRejectsInvalidRequests(t *testing.T) {
	runner, err := New(WithMode(ModeRemediation), AllowedSystemServices([]SystemServiceControlGrant{
		{
			Service: "mysql.service",
			Actions: []SystemServiceAction{SystemServiceRead},
		},
	}))
	require.NoError(t, err)
	defer runner.Close()

	tooLongUnit := strings.Repeat("a", builtins.MaxSystemServiceNameBytes-len(".service")+1) + ".service"
	invalidUTF8Unit := string([]byte{'a', 'p', 'i', 0xff, '.', 's', 'e', 'r', 'v', 'i', 'c', 'e'})
	tests := []struct {
		name     string
		action   SystemServiceAction
		services []string
		needle   string
	}{
		{name: "unknown action", action: "freeze", services: []string{"mysql.service"}, needle: "unsupported systemd action"},
		{name: "configuration-only wildcard", action: SystemServiceAllActions, services: []string{"mysql.service"}, needle: "unsupported systemd action"},
		{name: "no services", action: SystemServiceRead, needle: "at least one system service"},
		{name: "runtime resource separator", action: SystemServiceRead, services: []string{"tenant:mysql.service"}, needle: "must not contain ':'"},
		{name: "runtime glob", action: SystemServiceRead, services: []string{"mysql*.service"}, needle: "glob pattern"},
		{name: "runtime backslash path", action: SystemServiceRead, services: []string{"..\\mysql.service"}, needle: "path separator"},
		{name: "runtime name too long", action: SystemServiceRead, services: []string{tooLongUnit}, needle: "exceeds 255 bytes"},
		{name: "runtime invalid utf8", action: SystemServiceRead, services: []string{invalidUTF8Unit}, needle: "must be valid UTF-8"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runner.authorizeSystemServices(test.action, test.services...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.needle)
		})
	}
}
