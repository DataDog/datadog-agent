// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// withSelection pins the three inputs GetServiceManagerType reads and restores them afterwards.
func withSelection(t *testing.T, base Type, enabled, installed bool) {
	t.Helper()
	initSystemType = func() Type { return base }
	procmgrEnabled = func() bool { return enabled }
	procmgrInstalled = func(string) bool { return installed }
	t.Cleanup(func() {
		initSystemType = sync.OnceValue(detectInitSystem)
		procmgrEnabled = func() bool { return env.FromEnv().ProcessManagerEnabled }
		procmgrInstalled = isProcmgrInstalled
	})
}

func TestGetServiceManagerType(t *testing.T) {
	tests := []struct {
		name      string
		base      Type
		enabled   bool
		installed bool
		expected  Type
	}{
		{"systemd with procmgr installed and not opted out", SystemdType, true, true, ProcmgrType},
		{"systemd with procmgr opted out", SystemdType, false, true, SystemdType},
		{"systemd with procmgr not installed", SystemdType, true, false, SystemdType},
		{"systemd with procmgr opted out and not installed", SystemdType, false, false, SystemdType},
		{"upstart with procmgr installed and not opted out", UpstartType, true, true, ProcmgrType},
		{"upstart with procmgr opted out", UpstartType, false, true, UpstartType},
		{"upstart with procmgr not installed", UpstartType, true, false, UpstartType},
		{"sysvinit with procmgr installed and not opted out", SysvinitType, true, true, ProcmgrType},
		{"sysvinit with procmgr opted out", SysvinitType, false, true, SysvinitType},
		{"sysvinit with procmgr not installed", SysvinitType, true, false, SysvinitType},
		{"unknown is never overlaid", UnknownType, true, true, UnknownType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSelection(t, tt.base, tt.enabled, tt.installed)
			assert.Equal(t, tt.expected, GetServiceManagerType(""))
		})
	}
}

func TestInitSystemTypeIsNeverProcmgr(t *testing.T) {
	for _, base := range []Type{SystemdType, UpstartType, SysvinitType, UnknownType} {
		withSelection(t, base, true, true)
		assert.Equal(t, base, InitSystemType())
	}
}

// TestGetServiceManagerTypeIsNotMemoized guards the install-time transition: preInstall can run
// before dd-procmgrd is on disk while postInstall runs after, so the answer must be able to change
// within one process.
func TestGetServiceManagerTypeIsNotMemoized(t *testing.T) {
	installed := false
	withSelection(t, SystemdType, true, false)
	procmgrInstalled = func(string) bool { return installed }

	assert.Equal(t, SystemdType, GetServiceManagerType(""))
	installed = true
	assert.Equal(t, ProcmgrType, GetServiceManagerType(""))
}

// TestProcmgrIsTheDefault pins the point of expressing the setting as an opt-in that defaults to
// true: a zero-valued Env would select the plain init system, but a freshly constructed Env via FromEnv() (which
// is what production code calls) defaults to procmgr.
func TestProcmgrIsTheDefault(t *testing.T) {
	assert.True(t, env.FromEnv().ProcessManagerEnabled)
}
