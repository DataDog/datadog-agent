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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
)

func TestPrivilegedRshellUnit(t *testing.T) {
	units := unitSetPrivilegedRshell(stableDataOCI, expDataOCI, true)
	assert.Len(t, units, 2, "the helper creates its own socket: no socket unit")

	for name, want := range map[string]struct{ agent, installDir, etcDir string }{
		"datadog-agent-rshell-privileged.service":     {"datadog-agent.service", "/opt/datadog-packages/datadog-agent/stable", "/etc/datadog-agent"},
		"datadog-agent-rshell-privileged-exp.service": {"datadog-agent-exp.service", "/opt/datadog-packages/datadog-agent/experiment", "/etc/datadog-agent-exp"},
	} {
		unit := string(units[name])
		assert.Contains(t, unit, "PartOf="+want.agent+"\n", name)
		assert.NotContains(t, unit, "datadog-agent-action", name+": the helper must not depend on the PAR frontend")
		assert.NotContains(t, unit, "[Install]", name+": the Agent unit's Wants= pulls the helper in")
		// The socket lives outside /run/datadog, the Agent's RuntimeDirectory.
		assert.Contains(t, unit, "RuntimeDirectory=datadog-agent-rshell\n", name)
		assert.Contains(t, unit, "RuntimeDirectoryMode=0755\n", name)
		assert.Contains(t, unit, "EnvironmentFile=-"+want.etcDir+"/environment\n", name)
		assert.Contains(t, unit, "ExecStart="+want.installDir+"/embedded/bin/privateactionrunner privileged-helper"+
			" --cfgpath="+want.etcDir+"/datadog.yaml"+
			" --helper="+want.installDir+"/embedded/bin/rshell"+
			" --socket=/run/datadog-agent-rshell/privileged.sock"+
			" --policy=/etc/datadog-agent-rshell/policy.json\n", name)
		assert.Contains(t, unit, "Restart=on-failure\n", name)
	}
	assert.Contains(t, string(units["datadog-agent-rshell-privileged-exp.service"]), "After=datadog-agent-rshell-privileged.service\n",
		"the experiment helper must wait for the stable one to release the socket")
}

func TestAgentUnitsWantPrivilegedRshell(t *testing.T) {
	for _, procmgr := range []bool{false, true} {
		stable := string(mustReadUnit("datadog-agent.service", stableDataOCI, true, procmgr))
		experiment := string(mustReadUnit("datadog-agent.service", expDataOCI, true, procmgr))
		assert.Regexp(t, regexp.MustCompile(`(?m)^Wants=.* datadog-agent-rshell-privileged\.service( |$)`), stable)
		assert.Regexp(t, regexp.MustCompile(`(?m)^Wants=.* datadog-agent-rshell-privileged-exp\.service( |$)`), experiment)
		assert.Regexp(t, regexp.MustCompile(`(?m)^After=.* datadog-agent-rshell-privileged\.service$`), stable)
		assert.Regexp(t, regexp.MustCompile(`(?m)^After=.* datadog-agent-rshell-privileged-exp\.service$`), experiment)
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
