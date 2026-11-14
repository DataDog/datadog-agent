// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsermodDispatchGates(t *testing.T) {
	for _, test := range []struct {
		name, script, want          string
		remediation, allow, elevate bool
	}{
		{"readonly", "usermod --help", "remediation mode required", false, true, false},
		{"command", "usermod -aG docker dd-agent", "command not allowed", true, false, false},
		{"unelevated", "usermod -aG docker dd-agent", "authorized elevation", true, true, false},
		{"no elevation grant", "sudo usermod -aG docker dd-agent", "elevation not allowed", true, true, false},
		{"no path grant", "sudo usermod -aG docker dd-agent", "", true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			options := []RunnerOption{StdIO(nil, &out, &stderr), AllowedPaths([]string{t.TempDir() + ":rw"})}
			if test.allow {
				options = append(options, AllowedCommands([]string{"rshell:usermod"}))
			}
			if test.remediation {
				options = append(options, WithMode(ModeRemediation))
			}
			if test.elevate {
				options = append(options, SelectiveElevation([]string{"rshell:usermod"}, func(_ context.Context, _ string, run func()) error { run(); return nil }))
			}
			runner, err := New(options...)
			require.NoError(t, err)
			defer runner.Close()
			script, err := ParseScript(test.script, "")
			require.NoError(t, err)
			require.Error(t, runner.Run(context.Background(), script))
			if test.want != "" {
				require.Contains(t, stderr.String(), test.want)
			} else {
				require.NotEmpty(t, stderr.String())
			}
		})
	}
}

func TestUsermodNestedDispatchGates(t *testing.T) {
	for _, test := range []struct {
		name, script, want string
		childGrant         bool
	}{
		{"missing child elevation", "sudo xargs usermod -aG docker dd-agent", "elevation not allowed", false},
		{"unelevated child", "xargs usermod -aG docker dd-agent", "authorized elevation", true},
		{"elevated child without path", "sudo xargs usermod -aG docker dd-agent", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			grants := []string{"rshell:xargs"}
			if test.childGrant {
				grants = append(grants, "rshell:usermod")
			}
			runner, err := New(StdIO(nil, &stdout, &stderr), WithMode(ModeRemediation), AllowedPaths([]string{t.TempDir() + ":rw"}),
				AllowedCommands([]string{"rshell:xargs", "rshell:usermod"}),
				SelectiveElevation(grants, func(_ context.Context, _ string, run func()) error { run(); return nil }))
			require.NoError(t, err)
			defer runner.Close()
			script, err := ParseScript(test.script, "")
			require.NoError(t, err)
			require.Error(t, runner.Run(context.Background(), script))
			require.NotEmpty(t, stderr.String())
			if test.want != "" {
				require.Contains(t, stderr.String(), test.want)
			} else {
				require.NotContains(t, stderr.String(), "authorized elevation")
			}
		})
	}
}
