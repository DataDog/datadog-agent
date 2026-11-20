// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssertMacosReportsAbsence(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		marker string
		ok     bool
	}{
		{name: "launchctl missing", err: fmt.Errorf("Could not find service \"com.datadoghq.procmgr\" in domain for system: Process exited with status 113"), marker: "Could not find service", ok: true},
		{name: "status socket missing", err: fmt.Errorf("failed to connect to /var/run/datadog-procmgrd/dd-procmgrd.sock: refused: Process exited with status 1"), marker: "failed to connect to", ok: true},
		{name: "ssh failed", err: fmt.Errorf("failed to create session: dial tcp"), marker: "Could not find service", ok: false},
		{name: "binary missing", err: fmt.Errorf("sudo: dd-procmgr: command not found: Process exited with status 1"), marker: "failed to connect to", ok: false},
		{name: "command succeeded", marker: "Could not find service", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorded := &absenceFailure{}
			assertMacosReportsAbsence(recorded, "", tt.err, tt.marker)
			assert.Equal(t, tt.ok, !recorded.failed)
		})
	}
}

// fakeExitError stands in for the SSH exit error, which has no exported constructor.
type fakeExitError struct {
	status int
}

func (e *fakeExitError) Error() string {
	return fmt.Sprintf("Process exited with status %d", e.status)
}

func (e *fakeExitError) ExitStatus() int {
	return e.status
}

func TestAssertMacosExitStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		ok   bool
	}{
		{name: "status 1", err: fmt.Errorf(": %w", &fakeExitError{status: 1}), ok: true},
		{name: "status 126", err: fmt.Errorf(": %w", &fakeExitError{status: 126}), ok: false},
		{name: "signal death", err: fmt.Errorf(": %w", &fakeExitError{status: 137}), ok: false},
		{name: "untyped text with status 1", err: fmt.Errorf(": Process exited with status 1"), ok: false},
		{name: "sudo status 1", err: fmt.Errorf("sudo: a password is required: %w", &fakeExitError{status: 1}), ok: false},
		{name: "ssh failed", err: fmt.Errorf("failed to create session: %w", &fakeExitError{status: 1}), ok: false},
		{name: "command succeeded", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorded := &absenceFailure{}
			assertMacosExitStatus(recorded, "", tt.err, 1)
			assert.Equal(t, tt.ok, !recorded.failed)
		})
	}
}

// absenceFailure records whether an assertion failed. assert.CollectT keeps
// that state unexported.
type absenceFailure struct {
	failed bool
}

func (f *absenceFailure) Errorf(string, ...interface{}) {
	f.failed = true
}
