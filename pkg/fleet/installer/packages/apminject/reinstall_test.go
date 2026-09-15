// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apminject

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// instrumentedDockerDaemon is what setDockerConfigContent writes; the reverted
// variant is the daemon.json a `dd-container-install --uninstall` leaves behind.
const (
	instrumentedDockerDaemon = `{
    "default-runtime": "dd-shim",
    "runtimes": {
        "dd-shim": {
            "path": "/opt/datadog-packages/datadog-apm-inject/stable/inject/auto_inject_runc"
        }
    }
}`
	revertedDockerDaemon = `{
    "default-runtime": "runc",
    "log-driver": "json-file",
    "runtimes": {}
}`
)

func testInstaller(method string) *InjectorInstaller {
	a := NewInstaller()
	a.Env = &env.Env{}
	a.Env.InstallScript.APMInstrumentationEnabled = method
	return a
}

func TestIsDockerInstrumented(t *testing.T) {
	a := testInstaller(env.APMInstrumentationEnabledDocker)

	assert.True(t, a.isDockerInstrumented([]byte(instrumentedDockerDaemon)))
	assert.False(t, a.isDockerInstrumented([]byte(revertedDockerDaemon)))
	assert.False(t, a.isDockerInstrumented(nil))
	// Only the runtime entry is left: containers still start with runc.
	assert.False(t, a.isDockerInstrumented([]byte(`{"default-runtime":"runc","runtimes":{"dd-shim":{"path":"/x"}}}`)))
	// Content we cannot interpret must not drive a reinstall on every run.
	assert.True(t, a.isDockerInstrumented([]byte(`{"default-runtime":`)))
}

func TestIsHostInstrumented(t *testing.T) {
	a := testInstaller(env.APMInstrumentationEnabledHost)

	assert.True(t, a.isHostInstrumented([]byte(filepath.Join(injectorPath, "inject", "launcher.preload.so")+"\n")))
	assert.True(t, a.isHostInstrumented([]byte(filepath.Join(injectorPath, "inject", "$LIB", "launcher.preload.so")+"\n")))
	assert.True(t, a.isHostInstrumented([]byte(filepath.Join(defaultTmpfsInjectDir, "launcher.preload.so")+"\n")))
	assert.True(t, a.isHostInstrumented([]byte(oldLauncherPath+"\n")))
	assert.False(t, a.isHostInstrumented(nil))
	assert.False(t, a.isHostInstrumented([]byte("/usr/lib/other.so\n")))
}

func TestRequiresReinstall(t *testing.T) {
	persistentLauncher := filepath.Join(injectorPath, "inject", "launcher.preload.so") + "\n"
	tmpfsLauncher := filepath.Join(defaultTmpfsInjectDir, "launcher.preload.so") + "\n"

	// instrumented is the wiring a successful install leaves behind: nothing to
	// repair. Each case starts from it and reverts one part.
	instrumented := hostWiring{
		ldSoPreload:     []byte(persistentLauncher),
		dockerDaemon:    []byte(instrumentedDockerDaemon),
		dockerInstalled: true,
		installerPath:   "/opt/datadog-packages/run/datadog-installer-ssi",
		tmpfsCompatible: true,
	}

	tests := []struct {
		name     string
		method   string
		wiring   func(w hostWiring) hostWiring
		expected bool
	}{
		{
			name:   "fully instrumented host",
			method: env.APMInstrumentationEnabledAll,
			wiring: func(w hostWiring) hostWiring { return w },
		},
		{
			// APMS-20523: dd-container-install --uninstall reverts daemon.json
			// but leaves the package registered as installed.
			name:     "docker instrumentation was reverted",
			method:   env.APMInstrumentationEnabledDocker,
			wiring:   func(w hostWiring) hostWiring { w.dockerDaemon = []byte(revertedDockerDaemon); return w },
			expected: true,
		},
		{
			name:     "daemon.json was deleted",
			method:   env.APMInstrumentationEnabledAll,
			wiring:   func(w hostWiring) hostWiring { w.dockerDaemon = nil; return w },
			expected: true,
		},
		{
			name:   "docker instrumentation was reverted but docker is gone",
			method: env.APMInstrumentationEnabledDocker,
			wiring: func(w hostWiring) hostWiring {
				w.dockerDaemon = []byte(revertedDockerDaemon)
				w.dockerInstalled = false
				return w
			},
		},
		{
			name:   "docker instrumentation was reverted but only host is requested",
			method: env.APMInstrumentationEnabledHost,
			wiring: func(w hostWiring) hostWiring { w.dockerDaemon = []byte(revertedDockerDaemon); return w },
		},
		{
			// dd-host-install --uninstall, the host-side twin of the above.
			name:     "host instrumentation was reverted",
			method:   env.APMInstrumentationEnabledHost,
			wiring:   func(w hostWiring) hostWiring { w.ldSoPreload = nil; return w },
			expected: true,
		},
		{
			name:   "host instrumentation was reverted but only docker is requested",
			method: env.APMInstrumentationEnabledDocker,
			wiring: func(w hostWiring) hostWiring { w.ldSoPreload = nil; return w },
		},
		{
			name:   "host is instrumented through the tmpfs symlink",
			method: env.APMInstrumentationEnabledHost,
			wiring: func(w hostWiring) hostWiring { w.ldSoPreload = []byte(tmpfsLauncher); return w },
		},
		{
			name:   "tmpfs preload entry outlives its installer",
			method: env.APMInstrumentationEnabledHost,
			wiring: func(w hostWiring) hostWiring {
				w.ldSoPreload = []byte(tmpfsLauncher)
				w.tmpfsCompatible = false
				return w
			},
			expected: true,
		},
		{
			name:   "tmpfs preload entry with no installer to repair it",
			method: env.APMInstrumentationEnabledHost,
			wiring: func(w hostWiring) hostWiring {
				w.ldSoPreload = []byte(tmpfsLauncher)
				w.tmpfsCompatible = false
				w.installerPath = ""
				return w
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			required, reason := requiresReinstall(testInstaller(test.method), test.wiring(instrumented))
			assert.Equal(t, test.expected, required)
			assert.Equal(t, test.expected, reason != "", "a reinstall must always come with a reason")
		})
	}
}
