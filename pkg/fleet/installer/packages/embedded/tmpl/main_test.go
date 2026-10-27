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

func TestPrivilegedRshellSingleService(t *testing.T) {
	for _, layout := range systemdEmbeddedLayouts {
		if !strings.HasPrefix(layout.subdir, "r/") {
			continue
		}
		assert.Len(t, layout.units, 2, "only stable and experiment services are shipped")
		for name, content := range layout.units {
			t.Run(layout.subdir+"/"+name, func(t *testing.T) {
				unit := string(content)
				core, configDir := "datadog-agent.service", "/etc/datadog-agent"
				if strings.HasSuffix(name, "-exp.service") {
					core, configDir = "datadog-agent-exp.service", "/etc/datadog-agent-exp"
					assert.Contains(t, unit, "\nAfter=datadog-agent-rshell-privileged.service\n")
				}
				assert.Contains(t, unit, "\nBindsTo="+core+"\n")
				assert.Contains(t, unit, "\nBefore=datadog-agent.service datadog-agent-exp.service\n")
				assert.Regexp(t, `(?m)^ExecStart=\S+/embedded/bin/rshell privileged-helper `, unit)
				assert.Contains(t, unit, "--config="+configDir+"/datadog.yaml")
				assert.Contains(t, unit, "EnvironmentFile=-"+configDir+"/environment")
				assert.Contains(t, unit, `Environment="DD_FLEET_POLICIES_DIR=`+configDir+`/managed/datadog-agent/stable"`)
				assert.Contains(t, unit, "--socket=/run/datadog-agent-rshell/privileged.sock")
				assert.Contains(t, unit, "--policy=/etc/datadog-agent-rshell/policy.json")
				assert.Contains(t, unit, "--idle-timeout=0\n")
				assert.Contains(t, unit, "\nRuntimeDirectory=datadog-agent-rshell\nRuntimeDirectoryMode=0755\n")
				assert.Contains(t, unit, "\nRestart=on-failure\n")
				assert.NotContains(t, unit, "\nUser=", "the helper starts as root")
				for _, forbidden := range []string{"datadog-agent-action", "privateactionrunner", "ExecStartPre=", "[Install]", ".socket", "/bin/sh"} {
					assert.NotContains(t, unit, forbidden)
				}
			})
		}
	}
}

func TestCoreAgentWantsPrivilegedRshellService(t *testing.T) {
	for _, layouts := range [][]embeddedLayout{systemdEmbeddedLayouts, procmgrEmbeddedLayouts} {
		for _, layout := range layouts {
			for _, variant := range []string{"", "-exp"} {
				name := "datadog-agent" + variant + ".service"
				content, ok := layout.units[name]
				if !ok {
					continue
				}
				t.Run(layout.subdir+"/"+name, func(t *testing.T) {
					assert.Regexp(t, `(?m)^Wants=.*datadog-agent-rshell-privileged`+variant+`\.service(?: |$)`, string(content))
					assert.NotContains(t, string(content), "rshell-privileged"+variant+".socket")
				})
			}
		}
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
