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
	for name, socket := range map[string]string{"stable": stableSocket, "experiment": experimentSocket} {
		// Outside /run/datadog, the Agent's RuntimeDirectory, so the socket
		// keeps its default dependencies without an ordering cycle.
		assert.Equal(t, 1, strings.Count(socket, "ListenStream=/run/datadog-agent-rshell/privileged.sock"), name)
		assert.Contains(t, socket, "DirectoryMode=0755", name)
		assert.NotContains(t, socket, "DefaultDependencies=", name)
		assert.NotContains(t, socket, "[Install]", name+": the Agent unit's Wants= pulls the socket in")
		assert.NotContains(t, socket, "BindsTo=", name)
		assert.NotContains(t, socket, "datadog-agent-action", name+": the socket must not depend on the PAR frontend")
	}
	assert.Contains(t, stableSocket, "PartOf=datadog-agent.service\n")
	assert.Contains(t, experimentSocket, "PartOf=datadog-agent-exp.service\n")
	assert.Contains(t, experimentSocket, "After=datadog-agent-rshell-privileged.socket\n")

	for name, want := range map[string]struct{ agent, check string }{
		"datadog-agent-rshell-privileged.service":     {"datadog-agent.service", "datadog-agent-rshell-privileged-check.service"},
		"datadog-agent-rshell-privileged-exp.service": {"datadog-agent-exp.service", "datadog-agent-rshell-privileged-check-exp.service"},
	} {
		unit := string(units[name])
		check := want.check
		assert.Contains(t, unit, "PartOf="+want.agent+"\n", name)
		assert.NotContains(t, unit, "BindsTo=", name)
		// The helper does no work unless the configuration enables it.
		assert.Contains(t, unit, "Requires="+check+"\n", name)
		assert.Contains(t, unit, "After="+check+"\n", name)
	}

	for name, etcDir := range map[string]string{
		"datadog-agent-rshell-privileged-check.service":     "/etc/datadog-agent",
		"datadog-agent-rshell-privileged-check-exp.service": "/etc/datadog-agent-exp",
	} {
		unit := string(units[name])
		assert.Contains(t, unit, "Type=oneshot\n", name)
		assert.Contains(t, unit, "User=dd-agent\n", name+": loading the config can run the secret backend, so never as root")
		assert.Contains(t, unit, "privateactionrunner privileged-helper-enabled --cfgpath="+etcDir+"/datadog.yaml\n", name)
		assert.Contains(t, unit, "EnvironmentFile=-"+etcDir+"/environment\n", name)
	}
}

func TestAgentUnitsWantPrivilegedRshellSocket(t *testing.T) {
	for _, procmgr := range []bool{false, true} {
		stable := string(mustReadUnit("datadog-agent.service", stableDataOCI, true, procmgr))
		experiment := string(mustReadUnit("datadog-agent.service", expDataOCI, true, procmgr))
		assert.Regexp(t, regexp.MustCompile(`(?m)^Wants=.* datadog-agent-rshell-privileged\.socket( |$)`), stable)
		assert.Regexp(t, regexp.MustCompile(`(?m)^Wants=.* datadog-agent-rshell-privileged-exp\.socket( |$)`), experiment)
		// The socket listens before the runner can receive tasks.
		assert.Regexp(t, regexp.MustCompile(`(?m)^After=.* datadog-agent-rshell-privileged\.socket$`), stable)
		assert.Regexp(t, regexp.MustCompile(`(?m)^After=.* datadog-agent-rshell-privileged-exp\.socket$`), experiment)
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

// TestProcessesEnvVarsDefinedInProcmgrService tests that every environment
// variable referenced in a gen/pm/processes.d/*.yaml process definition
// is defined in every gen/pm/*/datadog-agent-procmgr.service context that
// starts the process manager responsible for running that process.
func TestParControlUsesActiveConfigDirForFleetPolicies(t *testing.T) {
	process, err := fs.ReadFile(genFS, "gen/pm/processes.d/datadog-agent-par-control.yaml")
	assert.NoError(t, err)
	assert.Contains(t, string(process), "DD_FLEET_POLICIES_DIR: ${DD_CONF_DIR}/managed/datadog-agent/stable")
}

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
