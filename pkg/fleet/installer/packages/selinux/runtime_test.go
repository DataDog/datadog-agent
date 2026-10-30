// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package selinux

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimePermissions(t *testing.T) {
	for _, tc := range []struct {
		name, state, existing, fail, wantError string
		persistent, chcon, fallbackDetection   bool
		wantOperation                          string
	}{
		{name: "enforcing", state: "Enforcing", persistent: true, wantOperation: "-a"},
		{name: "permissive", state: "Permissive", persistent: true, wantOperation: "-a"},
		{name: "existing", state: "Enforcing", persistent: true, existing: "var_run_t"},
		{name: "repair", state: "Enforcing", persistent: true, existing: "usr_t", wantOperation: "-m"},
		{name: "disabled", state: "Disabled"},
		{name: "chcon", state: "Enforcing", chcon: true},
		{name: "fallback detection", fallbackDetection: true, chcon: true},
		{name: "missing detection", wantError: "neither getenforce"},
		{name: "unknown state", state: "unknown", wantError: "unknown SELinux state"},
		{name: "missing tools", state: "Enforcing", wantError: "install semanage"},
		{name: "registration failure", state: "Enforcing", persistent: true, fail: "semanage", wantError: "semanage"},
		{name: "restore failure", state: "Enforcing", persistent: true, fail: "restorecon", wantError: "restorecon", wantOperation: "-a"},
		{name: "chcon failure", state: "Enforcing", chcon: true, fail: "chcon", wantError: "chcon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			calls := filepath.Join(t.TempDir(), "calls")
			require.NoError(t, os.WriteFile(calls, nil, 0600))
			t.Setenv("PATH", bin)
			t.Setenv("CALLS", calls)
			run := filepath.Join(t.TempDir(), "run.1")
			require.NoError(t, os.Mkdir(run, 0755))
			run, err := filepath.EvalSymlinks(run)
			require.NoError(t, err)
			link := filepath.Join(t.TempDir(), "stable-run")
			require.NoError(t, os.Symlink(run, link))
			rule := runtimeContextRule(run)
			write := func(name, body string) {
				require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0755))
			}
			if tc.state != "" {
				write("getenforce", "echo '"+tc.state+"'\n")
			}
			if tc.fallbackDetection {
				write("selinuxenabled", "exit 0\n")
			}
			t.Setenv("FAIL", tc.fail)
			body := "echo \"${0##*/} $*\" >> \"$CALLS\"\n[ \"$FAIL\" != \"${0##*/}\" ] || exit 2\n"
			if tc.persistent {
				listing := ""
				if tc.existing != "" {
					listing = rule + " all files system_u:object_r:" + tc.existing + ":s0"
				}
				write("semanage", body+"if [ \"$2\" = '-l' ]; then echo '"+listing+"'; fi\n")
				write("restorecon", body)
			}
			if tc.chcon {
				write("chcon", body)
			}
			err = SetRuntimePermissions(context.Background(), link)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
			output, err := os.ReadFile(calls)
			require.NoError(t, err)
			if tc.wantOperation != "" {
				require.Contains(t, string(output), "fcontext "+tc.wantOperation+" -t var_run_t "+rule)
			}
			if tc.existing == "var_run_t" {
				require.NotContains(t, string(output), "fcontext -a")
				require.NotContains(t, string(output), "fcontext -m")
			}
			if tc.persistent && tc.fail != "semanage" {
				require.Contains(t, string(output), "restorecon -R "+run)
			}
			if tc.chcon {
				require.Contains(t, string(output), "chcon -R -t var_run_t "+run)
			}
			if tc.state == "Disabled" {
				require.Empty(t, string(output))
			}
		})
	}
}

func TestRuntimeContextRule(t *testing.T) {
	require.Equal(t, "/opt/datadog-packages/datadog-agent/[^/]+/run(/.*)?", runtimeContextRule("/opt/datadog-packages/datadog-agent/7.80.0-1/run"))
	require.Equal(t, `/opt/custom\.agent/run(/.*)?`, runtimeContextRule("/opt/custom.agent/run"))
	require.Equal(t, "/opt/datadog-packages/run(/.*)?", runtimeContextRule("/opt/datadog-packages/run"))
}

func TestRuntimeSELinuxDetection(t *testing.T) {
	for _, tc := range []struct {
		name, program, script string
		enabled               bool
		wantError             bool
	}{
		{"getenforce failure", "getenforce", "exit 2", false, true},
		{"fallback disabled", "selinuxenabled", "exit 1", false, false},
		{"fallback failure", "selinuxenabled", "exit 2", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			require.NoError(t, os.WriteFile(filepath.Join(bin, tc.program), []byte("#!/bin/sh\n"+tc.script+"\n"), 0755))
			enabled, err := runtimeSELinuxEnabled(context.Background())
			require.Equal(t, tc.enabled, enabled)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
