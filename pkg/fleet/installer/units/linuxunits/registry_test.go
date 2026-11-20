// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package linuxunits

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
)

var flavors = []Flavor{
	{Install: OCI, AmbientCaps: true},
	{Install: OCI, AmbientCaps: false},
	{Install: DEB, AmbientCaps: true},
	{Install: DEB, AmbientCaps: false},
}

func content(t *testing.T, u units.Unit, f Flavor, exp bool) string {
	t.Helper()
	c, err := Content(u, f, exp)
	require.NoError(t, err)
	require.NotEmpty(t, c)
	return string(c)
}

func TestEveryUnitIsEmbeddedForEveryFlavor(t *testing.T) {
	for _, u := range All() {
		if u.Template == "" {
			continue
		}
		for _, f := range flavors {
			for _, exp := range []bool{false, true} {
				content(t, u, f, exp)
			}
		}
	}
}

func TestRPMUsesTheDEBRendering(t *testing.T) {
	assert.Equal(t, Path(Systemd.Agent, Flavor{Install: DEB}, false), Path(Systemd.Agent, Flavor{Install: RPM}, false))
}

func TestNocapFlavorDropsAmbientCapabilities(t *testing.T) {
	for _, install := range []Install{OCI, DEB} {
		assert.Contains(t, content(t, Systemd.Agent, Flavor{Install: install, AmbientCaps: true}, false), "AmbientCapabilities=")
		assert.NotContains(t, content(t, Systemd.Agent, Flavor{Install: install, AmbientCaps: false}, false), "AmbientCapabilities=")
	}
}

func TestProcmgrAgentUnitWantsProcmgrInsteadOfDDOT(t *testing.T) {
	f := Flavor{Install: OCI, AmbientCaps: true}
	assert.Contains(t, content(t, Procmgr.Agent, f, false), Procmgr.Procmgr.Name)
	assert.NotContains(t, content(t, Procmgr.Agent, f, false), Systemd.DDOT.Name)
	assert.Contains(t, content(t, Systemd.Agent, f, false), Systemd.DDOT.Name)
}

func TestStandaloneDDOTRunsFromItsOwnPackage(t *testing.T) {
	oci := content(t, Systemd.DDOTStandalone, Flavor{Install: OCI, AmbientCaps: true}, false)
	assert.Contains(t, oci, "ExecStart=/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent")
	assert.NotContains(t, oci, "/ext/ddot")
	deb := content(t, Systemd.DDOTStandalone, Flavor{Install: DEB, AmbientCaps: true}, true)
	assert.Contains(t, deb, "ExecStart=/opt/datadog-agent/embedded/bin/otel-agent")
}

func TestPrivilegedRshellUnitsUseProtectedPolicyAndConfiguredSocket(t *testing.T) {
	f := Flavor{Install: OCI, AmbientCaps: true}
	for _, exp := range []bool{false, true} {
		assert.Contains(t, content(t, Systemd.RshellService, f, exp), "--policy=/etc/datadog-agent-rshell/policy.json")
	}
	stableSocket := content(t, Systemd.RshellSocket, f, false)
	experimentSocket := content(t, Systemd.RshellSocket, f, true)
	assert.Contains(t, stableSocket, "WantedBy=sockets.target datadog-agent-action.service")
	assert.Contains(t, experimentSocket, "WantedBy=sockets.target datadog-agent-action-exp.service")
	assert.Equal(t, 1, strings.Count(stableSocket, "ListenStream=/run/datadog/rshell-privileged.sock"))
	assert.Equal(t, 1, strings.Count(experimentSocket, "ListenStream=/run/datadog/rshell-privileged.sock"))
	assert.NotContains(t, experimentSocket, "rshell-privileged-exp.sock")
}

func TestPARInitialStartupIsCoreOwned(t *testing.T) {
	for _, tc := range []struct {
		process units.Unit
		restart string
	}{
		{Procmgr.ParControl, "on-failure"},
		{Procmgr.ActionExecutor, "never"},
	} {
		process := content(t, tc.process, Flavor{}, false)
		assert.Contains(t, process, "\nauto_start: false\n")
		assert.Contains(t, process, "\nrestart: "+tc.restart+"\n")
	}
	control := content(t, Procmgr.ParControl, Flavor{}, false)
	assert.Contains(t, control, "\nrestart_sec: 2\n")
	assert.Contains(t, control, "\nruntime_success_sec: 60\n")
	assert.Contains(t, control, "\nrestart_max_delay_sec: 300\n")
	assert.Contains(t, control, "DD_FLEET_POLICIES_DIR: ${DD_CONF_DIR}/managed/datadog-agent/stable")
}

// TestProcessesEnvVarsDefinedInProcmgrService tests that every environment variable referenced by
// a processes.d entry is defined by every datadog-agent-procmgr.service starting it.
func TestProcessesEnvVarsDefinedInProcmgrService(t *testing.T) {
	for _, process := range units.Fields(Procmgr) {
		if !IsProcess(process) {
			continue
		}
		processContent := content(t, process, Flavor{}, false)
		selfDefined := map[string]bool{}
		for _, match := range regexp.MustCompile(`(?m)^  (\w+):`).FindAllStringSubmatch(processContent, -1) {
			selfDefined[match[1]] = true
		}
		for _, f := range flavors {
			for _, exp := range []bool{false, true} {
				service := content(t, Procmgr.Procmgr, f, exp)
				for _, match := range regexp.MustCompile(`\$\{(\w+)}`).FindAllStringSubmatch(processContent, -1) {
					if !selfDefined[match[1]] {
						assert.Contains(t, service, `Environment="`+match[1]+"=", "%s uses %s", process.Name, match[1])
					}
				}
			}
		}
	}
}
