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
	"os/exec"
	"path/filepath"
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

// The socket must not use systemd's default dependencies: they order it
// Before=sockets.target, while it is ordered After= the agent services, which
// are After=basic.target (itself After=sockets.target). That is an ordering
// cycle, and systemd breaks it at boot by dropping one of the start jobs.
func TestPrivilegedRshellSocketHasNoDefaultDependencies(t *testing.T) {
	for _, data := range []struct {
		name        string
		stable, exp installerTemplateData
	}{
		{"oci", stableDataOCI, expDataOCI},
		{"debrpm", stableDataDebRpm, expDataDebRpm},
	} {
		units := unitSetPrivilegedRshell(data.stable, data.exp, true)
		for _, socket := range []string{
			"datadog-agent-rshell-privileged.socket",
			"datadog-agent-rshell-privileged-exp.socket",
		} {
			t.Run(data.name+"/"+socket, func(t *testing.T) {
				content := string(units[socket])
				for _, line := range []string{
					"DefaultDependencies=no",
					"Requires=sysinit.target",
					"After=sysinit.target",
					"Before=shutdown.target",
					"Conflicts=shutdown.target",
				} {
					assert.Contains(t, content, "\n"+line+"\n")
				}
				assert.NotContains(t, content, "Before=sockets.target")
			})
		}
	}
}

// TestPrivilegedRshellUnitsHaveNoOrderingCycle loads the rendered units, enabled
// the way the installer enables them, into systemd-analyze and checks that
// starting multi-user.target does not hit an ordering cycle.
func TestPrivilegedRshellUnitsHaveNoOrderingCycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd-analyze is only available on Linux")
	}
	if _, err := exec.LookPath("systemd-analyze"); err != nil {
		t.Skip("systemd-analyze is not installed")
	}

	for _, data := range []struct {
		name        string
		stable, exp installerTemplateData
	}{
		{"oci", stableDataOCI, expDataOCI},
		{"debrpm", stableDataDebRpm, expDataDebRpm},
	} {
		units := unitSetSystemd(data.stable, data.exp, true)
		for name, content := range unitSetPrivilegedRshell(data.stable, data.exp, true) {
			units[name] = content
		}
		for _, suffix := range []string{"", "-exp"} {
			t.Run(data.name+suffix, func(t *testing.T) {
				socket := "datadog-agent-rshell-privileged" + suffix + ".socket"
				// Control: with systemd's default dependencies restored, the
				// cycle must be reported, or this systemd-analyze cannot detect it.
				withDefaults := strings.Replace(string(units[socket]), "DefaultDependencies=no\n", "", 1)
				if !strings.Contains(analyzePrivilegedRshellUnits(t, units, suffix, withDefaults), "ordering cycle") {
					t.Skip("systemd-analyze on this host does not report ordering cycles")
				}
				assert.NotContains(t, analyzePrivilegedRshellUnits(t, units, suffix, string(units[socket])), "ordering cycle")
			})
		}
	}
}

// analyzePrivilegedRshellUnits runs systemd-analyze verify on the agent, action
// and rshell units, with socketContent as the socket unit, and returns its output.
func analyzePrivilegedRshellUnits(t *testing.T, units map[string][]byte, suffix, socketContent string) string {
	dir := t.TempDir()
	agent := "datadog-agent" + suffix + ".service"
	action := "datadog-agent-action" + suffix + ".service"
	socket := "datadog-agent-rshell-privileged" + suffix + ".socket"
	service := "datadog-agent-rshell-privileged" + suffix + ".service"
	for _, name := range []string{agent, action, service} {
		assert.NoError(t, os.WriteFile(filepath.Join(dir, name), units[name], 0644))
	}
	assert.NoError(t, os.WriteFile(filepath.Join(dir, socket), []byte(socketContent), 0644))
	// Enable the units the way their [Install] sections do.
	for wants, unit := range map[string][]string{
		"multi-user.target.wants": {agent, action},
		"sockets.target.wants":    {socket},
		action + ".wants":         {socket},
	} {
		assert.NoError(t, os.MkdirAll(filepath.Join(dir, wants), 0755))
		for _, u := range unit {
			assert.NoError(t, os.Symlink(filepath.Join("..", u), filepath.Join(dir, wants, u)))
		}
	}
	cmd := exec.Command("systemd-analyze", "verify", "--man=no", filepath.Join(dir, socket), "multi-user.target")
	// The trailing separator appends the default unit search path.
	cmd.Env = append(os.Environ(), "SYSTEMD_UNIT_PATH="+dir+":")
	// systemd-analyze exits non-zero on unrelated findings, such as the
	// agent binaries not being installed on this host; only the output matters.
	out, _ := cmd.CombinedOutput()
	return string(out)
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
