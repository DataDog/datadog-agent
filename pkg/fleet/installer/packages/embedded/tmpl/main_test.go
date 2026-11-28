// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates the systemd units for the installer.
package main

import (
	"embed"
	"io/fs"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
)

func TestPrivilegedRshellUnitsUseProtectedPolicyAndConfiguredSocket(t *testing.T) {
	units := unitSetPrivilegedRshell(stableDataOCI, expDataOCI, true)

	for _, service := range []string{
		"datadog-agent-rshell-privileged.service",
		"datadog-agent-rshell-privileged-exp.service",
	} {
		assert.Contains(t, string(units[service]), "--policy=/etc/datadog-agent-rshell/policy.json")
	}

	stableSocket := string(units["datadog-agent-rshell-privileged.socket"])
	experimentSocket := string(units["datadog-agent-rshell-privileged-exp.socket"])
	assert.Contains(t, stableSocket, "WantedBy=sockets.target datadog-agent-action.service")
	assert.Contains(t, experimentSocket, "WantedBy=sockets.target datadog-agent-action-exp.service")
	assert.Equal(t, 1, strings.Count(stableSocket, "ListenStream=/run/datadog/rshell-privileged.sock"))
	assert.Equal(t, 1, strings.Count(experimentSocket, "ListenStream=/run/datadog/rshell-privileged.sock"))
	assert.NotContains(t, experimentSocket, "rshell-privileged-exp.sock")
}

// TestProcmgrUnitsStopOnlyTheDaemonWithEnoughTimeout checks that the procmgr units signal only
// dd-procmgrd on stop (KillMode=mixed), so it can stop its processes in dependency order, and
// that TimeoutStopSec covers a chain of dependent processes (TimeoutStopSec=360).
func TestProcmgrUnitsStopOnlyTheDaemonWithEnoughTimeout(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		stableData, expData          installerTemplateData
		ambiantCapabilitiesSupported bool
	}{
		{"oci", stableDataOCI, expDataOCI, true},
		{"debrpm", stableDataDebRpm, expDataDebRpm, true},
		{"oci-nc", stableDataOCI, expDataOCI, false},
		{"debrpm-nc", stableDataDebRpm, expDataDebRpm, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			units := unitSetProcmgr(tc.stableData, tc.expData, tc.ambiantCapabilitiesSupported)
			for _, unit := range []string{"datadog-agent-procmgr.service", "datadog-agent-procmgr-exp.service"} {
				content := string(units[unit])
				serviceStart := strings.Index(content, "\n[Service]\n")
				if !assert.NotEqual(t, -1, serviceStart, "%s has no [Service] section", unit) {
					continue
				}
				// KillMode and TimeoutStopSec are only honored in [Service], so stop at the next section header.
				service := content[serviceStart:]
				if end := strings.Index(service[1:], "\n["); end != -1 {
					service = service[:end+2]
				}
				assert.Contains(t, service, "\nKillMode=mixed\n", "%s must signal only dd-procmgrd on stop", unit)
				assert.Contains(t, service, "\nTimeoutStopSec=360\n", "%s must allow the dependency chain to stop", unit)
			}
		})
	}
}

//go:embed gen
var genFS embed.FS

// TestGenerationIsUpToDate tests that the generated templates are up to date.
//
// Regenerate the templates with:
// bazelisk run //pkg/fleet/installer/packages/embedded:tmpl -- "$PWD/pkg/fleet/installer/packages/embedded/tmpl/gen"
func TestGenerationIsUpToDate(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestGenerationIsUpToDate is known to fail on the macOS Gitlab runners.")
	}

	generated := t.TempDir()
	err := generate(generated)
	assert.NoError(t, err)

	newGeneratedFS := os.DirFS(generated)
	currentGeneratedFS, err := fs.Sub(genFS, "gen")
	assert.NoError(t, err)

	fixtures.AssertEqualFS(t, currentGeneratedFS, newGeneratedFS)
}

func TestPARInitialStartupIsCoreOwned(t *testing.T) {
	for _, tc := range []struct {
		name, restart string
	}{
		{"datadog-agent-par-control.yaml", "on-failure"},
		{"datadog-agent-action-executor.yaml", "never"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process, err := fs.ReadFile(genFS, "gen/pm/processes.d/"+tc.name)
			assert.NoError(t, err)
			assert.Contains(t, string(process), "\nauto_start: false\n")
			assert.Contains(t, string(process), "\nrestart: "+tc.restart+"\n")
		})
	}
	control, err := fs.ReadFile(genFS, "gen/pm/processes.d/datadog-agent-par-control.yaml")
	assert.NoError(t, err)
	assert.Contains(t, string(control), "\nrestart_sec: 2\n")
	assert.Contains(t, string(control), "\nruntime_success_sec: 60\n")
	assert.Contains(t, string(control), "\nrestart_max_delay_sec: 300\n")
}

func TestParControlUsesActiveConfigDirForFleetPolicies(t *testing.T) {
	process, err := fs.ReadFile(genFS, "gen/pm/processes.d/datadog-agent-par-control.yaml")
	assert.NoError(t, err)
	assert.Contains(t, string(process), "DD_FLEET_POLICIES_DIR: ${DD_CONF_DIR}/managed/datadog-agent/stable")
}

// TestProcessesEnvVarsDefinedInProcmgrService tests that every environment
// variable referenced in a gen/pm/processes.d/*.yaml process definition
// is defined in every gen/pm/*/datadog-agent-procmgr.service context that
// starts the process manager responsible for running that process.
func TestProcessesEnvVarsDefinedInProcmgrService(t *testing.T) {
	processesDir, err := fs.Sub(genFS, "gen/pm/processes.d")
	assert.NoError(t, err)

	processFiles, err := fs.ReadDir(processesDir, ".")
	assert.NoError(t, err)

	procmgrServices, err := fs.Glob(genFS, "gen/pm/*/datadog-agent-procmgr.service")
	assert.NoError(t, err)
	assert.NotEmpty(t, procmgrServices)

	definedInService := map[string]map[string]bool{}
	for _, servicePath := range procmgrServices {
		serviceContent, err := fs.ReadFile(genFS, servicePath)
		assert.NoError(t, err)

		vars := map[string]bool{}
		for _, match := range regexp.MustCompile(`Environment="(\w+)=`).FindAllStringSubmatch(string(serviceContent), -1) {
			vars[match[1]] = true
		}
		definedInService[servicePath] = vars
	}

	for _, processFile := range processFiles {
		processContent, err := fs.ReadFile(processesDir, processFile.Name())
		assert.NoError(t, err)

		referencedVars := map[string]bool{}
		for _, match := range regexp.MustCompile(`\$\{(\w+)}`).FindAllStringSubmatch(string(processContent), -1) {
			referencedVars[match[1]] = true
		}

		selfDefinedVars := map[string]bool{}
		for _, match := range regexp.MustCompile(`(?m)^  (\w+):`).FindAllStringSubmatch(string(processContent), -1) {
			selfDefinedVars[match[1]] = true
		}

		for v := range referencedVars {
			if selfDefinedVars[v] {
				continue
			}
			for servicePath, vars := range definedInService {
				assert.True(t, vars[v], "environment variable %q is used in %s but not defined in %s", v, processFile.Name(), servicePath)
			}
		}
	}
}
