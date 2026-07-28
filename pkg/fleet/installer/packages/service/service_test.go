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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/procmgr"
)

// withSelection pins the three inputs GetServiceManagerType reads and restores them afterwards.
func withSelection(t *testing.T, base Type, disabled, installed bool) {
	t.Helper()
	initSystemType = func() Type { return base }
	procmgrDisabled = func() bool { return disabled }
	procmgrInstalled = func() bool { return installed }
	t.Cleanup(func() {
		initSystemType = sync.OnceValue(detectInitSystem)
		procmgrDisabled = func() bool { return env.FromEnv().ProcmgrDisabled }
		procmgrInstalled = procmgr.IsInstalled
	})
}

func TestGetServiceManagerType(t *testing.T) {
	tests := []struct {
		name      string
		base      Type
		disabled  bool
		installed bool
		expected  Type
	}{
		{"systemd with procmgr installed and not opted out", SystemdType, false, true, ProcmgrType},
		{"systemd with procmgr opted out", SystemdType, true, true, SystemdType},
		{"systemd with procmgr not installed", SystemdType, false, false, SystemdType},
		{"systemd with procmgr opted out and not installed", SystemdType, true, false, SystemdType},
		{"upstart is never overlaid", UpstartType, false, true, UpstartType},
		{"sysvinit is never overlaid", SysvinitType, false, true, SysvinitType},
		{"unknown is never overlaid", UnknownType, false, true, UnknownType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSelection(t, tt.base, tt.disabled, tt.installed)
			assert.Equal(t, tt.expected, GetServiceManagerType())
		})
	}
}

// TestGetServiceManagerTypeIsNotMemoized guards the install-time transition: preInstall can run
// before dd-procmgrd is on disk while postInstall runs after, so the answer must be able to change
// within one process.
func TestGetServiceManagerTypeIsNotMemoized(t *testing.T) {
	installed := false
	withSelection(t, SystemdType, false, false)
	procmgrInstalled = func() bool { return installed }

	assert.Equal(t, SystemdType, GetServiceManagerType())
	installed = true
	assert.Equal(t, ProcmgrType, GetServiceManagerType())
}

// TestProcmgrIsTheDefault pins the point of expressing the setting as an opt-out: a zero-valued Env
// selects procmgr, so a partially constructed Env cannot silently fall back to plain systemd.
func TestProcmgrIsTheDefault(t *testing.T) {
	assert.False(t, (&env.Env{}).ProcmgrDisabled)
	assert.NotContains(t, (&env.Env{}).ToEnv(), "DD_PROCMGR_DISABLE=true")
}
