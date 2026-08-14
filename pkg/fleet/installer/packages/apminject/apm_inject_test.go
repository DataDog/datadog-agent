// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeServiceManager is a test double for systemdServiceManager that records
// Uninstall calls and returns canned results.
type fakeServiceManager struct {
	installerPath     string
	serviceFileExists bool
	setupErr          error
	uninstallCalls    int
}

func (f *fakeServiceManager) InstallerPath() string             { return f.installerPath }
func (f *fakeServiceManager) ServiceFileExists() bool           { return f.serviceFileExists }
func (f *fakeServiceManager) Setup(_ context.Context) error     { return f.setupErr }
func (f *fakeServiceManager) Uninstall(_ context.Context) error { f.uninstallCalls++; return nil }

// TestSetupSystemdPreloadUnit covers the fallback branches: the function must
// return true (use the tmpfs link) only when an installer is present AND Setup
// succeeds, and must clean up the unit on every failure path so a unit that
// cannot start is never left enabled.
func TestSetupSystemdPreloadUnit(t *testing.T) {
	tests := []struct {
		name              string
		installerPath     string
		serviceFileExists bool
		setupErr          error
		wantRunning       bool
		wantRollback      bool
		wantUninstall     int
	}{
		{
			name:          "no supported installer, nothing to clean up",
			installerPath: "",
			wantRunning:   false,
			wantRollback:  false,
			wantUninstall: 0,
		},
		{
			name:              "no supported installer, stale unit removed",
			installerPath:     "",
			serviceFileExists: true,
			wantRunning:       false,
			wantRollback:      false,
			wantUninstall:     1,
		},
		{
			name:          "setup succeeds, rollback registered",
			installerPath: "/usr/bin/datadog-installer",
			wantRunning:   true,
			wantRollback:  true,
			wantUninstall: 0,
		},
		{
			name:              "setup fails, unit cleaned up and falls back",
			installerPath:     "/usr/bin/datadog-installer",
			serviceFileExists: true,
			setupErr:          errors.New("start failed"),
			wantRunning:       false,
			wantRollback:      false,
			wantUninstall:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeServiceManager{
				installerPath:     tt.installerPath,
				serviceFileExists: tt.serviceFileExists,
				setupErr:          tt.setupErr,
			}
			a := &InjectorInstaller{}

			running := a.setupSystemdPreloadUnit(context.TODO(), fake)

			assert.Equal(t, tt.wantRunning, running, "serviceRunning return")
			assert.Equal(t, tt.wantUninstall, fake.uninstallCalls, "Uninstall call count")
			if tt.wantRollback {
				assert.Len(t, a.rollbacks, 1, "a rollback must be registered when the unit is running")
			} else {
				assert.Empty(t, a.rollbacks, "no rollback must be registered on the fallback paths")
			}
		})
	}
}

func TestSetLDPreloadConfig(t *testing.T) {
	a := &InjectorInstaller{
		installPath: "/tmp/stable",
	}
	testCases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "File doesn't exist",
			input:    nil,
			expected: []byte("/tmp/stable/inject/launcher.preload.so\n"),
		},
		{
			name:     "Don't reuse the input buffer",
			input:    make([]byte, 2, 1000),
			expected: append([]byte{0x0, 0x0}, []byte("\n/tmp/stable/inject/launcher.preload.so\n")...),
		},
		{
			name:     "File contains unrelated entries",
			input:    []byte("/abc/def/preload.so\n"),
			expected: []byte("/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n"),
		},
		{
			name:     "File contains unrelated entries with no newline",
			input:    []byte("/abc/def/preload.so"),
			expected: []byte("/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n"),
		},
		{
			name:     "File contains old preload instructions",
			input:    []byte("banana\n/opt/datadog/apm/inject/launcher.preload.so\ntomato"),
			expected: []byte("banana\n/tmp/stable/inject/launcher.preload.so\ntomato"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := a.setLDPreloadConfigContent(context.TODO(), tc.input)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected, output)
			if len(tc.input) > 0 {
				assert.False(t, &tc.input[0] == &output[0])
			}
		})
	}
}

func TestRemoveLDPreloadConfig(t *testing.T) {
	a := &InjectorInstaller{
		installPath: "/tmp/stable",
	}

	for input, expected := range map[string]string{
		// File doesn't exist
		"": "",
		// File only contains the entry to remove
		"/tmp/stable/inject/launcher.preload.so\n": "",
		// File only contains the entry to remove without newline
		"/tmp/stable/inject/launcher.preload.so": "",
		// File contains unrelated entries
		"/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n": "/abc/def/preload.so\n",
		// File contains unrelated entries at the end
		"/tmp/stable/inject/launcher.preload.so\n/def/abc/preload.so": "/def/abc/preload.so",
		// File contains multiple unrelated entries
		"/abc/def/preload.so\n/tmp/stable/inject/launcher.preload.so\n/def/abc/preload.so": "/abc/def/preload.so\n/def/abc/preload.so",
		// File contains unrelated entries with no newline (reformatted by customer?)
		"/abc/def/preload.so /tmp/stable/inject/launcher.preload.so": "/abc/def/preload.so",
		// File contains unrelated entries with no newline (reformatted by customer?)
		"/abc/def/preload.so /tmp/stable/inject/launcher.preload.so /def/abc/preload.so": "/abc/def/preload.so /def/abc/preload.so",
		// File contains unrelated entries with no newline (reformatted by customer?)
		"/tmp/stable/inject/launcher.preload.so /def/abc/preload.so": "/def/abc/preload.so",
		// File doesn't contain the entry to remove (removed by customer?)
		"/abc/def/preload.so /def/abc/preload.so": "/abc/def/preload.so /def/abc/preload.so",
		// File contains a dynamic entry
		"/tmp/stable/inject/$lib/launcher.preload.so": "",
	} {
		output, err := a.deleteLDPreloadConfigContent(context.TODO(), []byte(input))
		assert.Nil(t, err)
		assert.Equal(t, expected, string(output))
	}

}

func TestSetLDPreloadConfig_TmpfsMigratesPersistentPath(t *testing.T) {
	// When the active entry is the tmpfs symlink path, a stale persistent OCI
	// entry must be migrated in place (not left behind, which would re-create
	// the reboot hazard).
	a := &InjectorInstaller{
		installPath:    "/opt/datadog-packages/datadog-apm-inject/stable",
		tmpfsInjectDir: "/run/datadog-apm-inject",
		launcherPath:   "/run/datadog-apm-inject/launcher.preload.so",
	}

	out, err := a.setLDPreloadConfigContent(context.TODO(),
		[]byte("/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so\n"))
	require.NoError(t, err)
	assert.Equal(t, "/run/datadog-apm-inject/launcher.preload.so\n", string(out))

	// Idempotent: the tmpfs entry already present returns unchanged.
	out, err = a.setLDPreloadConfigContent(context.TODO(), out)
	require.NoError(t, err)
	assert.Equal(t, "/run/datadog-apm-inject/launcher.preload.so\n", string(out))
}

func TestRemoveLDPreloadConfig_TmpfsPath(t *testing.T) {
	a := &InjectorInstaller{
		installPath:    "/opt/datadog-packages/datadog-apm-inject/stable",
		tmpfsInjectDir: "/run/datadog-apm-inject",
	}
	for input, expected := range map[string]string{
		"/run/datadog-apm-inject/launcher.preload.so\n":                              "",
		"/abc/def/preload.so\n/run/datadog-apm-inject/launcher.preload.so\n":         "/abc/def/preload.so\n",
		"/run/datadog-apm-inject/$lib/launcher.preload.so":                           "",
		"/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so": "",
	} {
		output, err := a.deleteLDPreloadConfigContent(context.TODO(), []byte(input))
		assert.NoError(t, err)
		assert.Equal(t, expected, string(output))
	}
}

func TestShouldInstrumentHost(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"Instrument all", "all", true},
		{"Instrument host", "host", true},
		{"Instrument docker", "docker", false},
		{"Invalid value", "unknown", false},
		{"not set", "not_set", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execEnvs := &env.Env{
				InstallScript: env.InstallScriptEnv{
					APMInstrumentationEnabled: tt.envValue,
				},
			}
			result := shouldInstrumentHost(execEnvs)
			if result != tt.expected {
				t.Errorf("shouldInstrumentHost() with envValue %s; got %t, want %t", tt.envValue, result, tt.expected)
			}
		})
	}
}

func TestVerifySharedLib_Missing(t *testing.T) {
	a := &InjectorInstaller{
		installPath: "/nonexistent/path",
	}
	err := a.verifySharedLib(context.TODO(), "/nonexistent/path/launcher.preload.so")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "launcher library not found")
}

func TestInstrumentLDPreload_MissingLibrary(t *testing.T) {
	tmpDir := t.TempDir()
	preloadFile := filepath.Join(tmpDir, "ld.so.preload")

	a := newInstallerWithPaths(tmpDir, preloadFile)

	err := a.InstrumentLDPreload(context.TODO(), ViaPersistentPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "launcher library not found")

	// ld.so.preload must not have been created
	_, statErr := os.Stat(preloadFile)
	assert.True(t, os.IsNotExist(statErr), "ld.so.preload must not be created on failure")
}

func TestUninstrumentLDPreload_NoPreloadFile(t *testing.T) {
	tmpDir := t.TempDir()
	preloadFile := filepath.Join(tmpDir, "ld.so.preload")

	a := newInstallerWithPaths(tmpDir, preloadFile)

	// File doesn't exist: UninstrumentLDPreload should succeed (idempotent)
	err := a.UninstrumentLDPreload(context.TODO())
	assert.NoError(t, err)
}

func TestUninstrumentLDPreload_RemovesEntry(t *testing.T) {
	tmpDir := t.TempDir()
	preloadFile := filepath.Join(tmpDir, "ld.so.preload")
	launcherPath := filepath.Join(tmpDir, "inject", "launcher.preload.so")

	err := os.WriteFile(preloadFile, []byte(launcherPath+"\n"), 0644)
	require.NoError(t, err)

	a := newInstallerWithPaths(tmpDir, preloadFile)

	err = a.UninstrumentLDPreload(context.TODO())
	assert.NoError(t, err)

	content, err := os.ReadFile(preloadFile)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "launcher.preload.so")
}

// newInstallerWithPaths creates an InjectorInstaller using the given install path and preload file
// path, suitable for unit testing without touching real system files.
func newInstallerWithPaths(installPath, preloadPath string) *InjectorInstaller {
	a := &InjectorInstaller{
		installPath: installPath,
	}
	a.ldPreloadFileInstrument = newFileMutator(preloadPath, a.setLDPreloadConfigContent, nil, nil)
	a.ldPreloadFileUninstrument = newFileMutator(preloadPath, a.deleteLDPreloadConfigContent, nil, nil)
	return a
}

func TestShouldInstrumentDocker(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"Instrument all", "all", true},
		{"Instrument host", "host", false},
		{"Instrument docker", "docker", true},
		{"Invalid value", "unknown", false},
		{"not set", "not_set", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execEnvs := &env.Env{
				InstallScript: env.InstallScriptEnv{
					APMInstrumentationEnabled: tt.envValue,
				},
			}
			result := shouldInstrumentDocker(execEnvs)
			if result != tt.expected {
				t.Errorf("shouldInstrumentDocker() with envValue %s; got %t, want %t", tt.envValue, result, tt.expected)
			}
		})
	}
}
