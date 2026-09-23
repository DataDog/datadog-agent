// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	processProcessName           = "datadog-agent-process"
	processLegacySCMServiceName  = "datadog-process-agent"
	processProcmgrConfigFileName = "datadog-agent-process.yaml"

	// The provisioned datadog.yaml leaves log_level at its info default, so a running
	// process-agent can only report this level if it came through the merged environment.
	legacySCMLogLevel = "warn"
)

type processProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestProcessAgentManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &processProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(),
			),
		),
	))
}

// TestProcessAgentSupervisedByProcmgrAndLegacySCMStopped is the end-to-end proof of the
// Windows cutover: on a default install dd-procmgr brings process-agent up on its own, and
// the core Agent leaves the legacy SCM service alone. Both halves have to hold at once.
// Either one alone is a bug: only the first means two process-agents, only the second means
// none at all.
func (s *processProcmgrWindowsSuite) TestProcessAgentSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	// Unlike PAR, process-agent ships with every install, so a missing binary is a failure
	// rather than a reason to skip.
	processBin := filepath.Join(installRoot, "bin", "agent", "process-agent.exe")
	exists, err := host.FileExists(processBin)
	require.NoError(s.T(), err)
	require.True(s.T(), exists, "process-agent.exe should be installed at %s", processBin)

	cfg := filepath.Join(installRoot, "processes.d", processProcmgrConfigFileName)
	exists, err = host.FileExists(cfg)
	require.NoError(s.T(), err)
	require.True(s.T(), exists, "fleet process-agent processes.d config should exist at %s", cfg)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processProcessName))
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
	}, 120*time.Second, 3*time.Second)

	out, err := host.Execute(fmt.Sprintf(
		`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`,
		processLegacySCMServiceName,
	))
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), "Running", strings.TrimSpace(out),
		"%s Windows service must not be Running when process-agent is managed by dd-procmgr", processLegacySCMServiceName)
}

// TestProcessAgentInheritsFilteredLegacyScmEnvironment covers the environment hand-off that
// makes the cutover transparent: process-agent used to run as an SCM service and pick up the
// Environment block configured on it, so dd-procmgr merges that same block when it spawns the
// process. The denylist matters as much as the merge. procmgr resolves the fleet policy
// directory itself, and inheriting a stale DD_FLEET_POLICIES_DIR from the legacy service would
// point process-agent at the wrong policies.
func (s *processProcmgrWindowsSuite) TestProcessAgentInheritsFilteredLegacyScmEnvironment() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)

	cli := joinWindowsPath(installRoot, "bin", "agent", "dd-procmgr.exe")

	s.T().Cleanup(func() {
		if _, err := host.Execute(psClearServiceEnvironment(processLegacySCMServiceName)); err != nil {
			s.T().Logf("failed to clear the %s Environment value: %v", processLegacySCMServiceName, err)
		}
		if _, err := host.Execute(procmgrRespawn(cli, processProcessName)); err != nil {
			s.T().Logf("failed to respawn %s: %v", processProcessName, err)
		}
	})

	_, err = host.Execute(psSetServiceEnvironment(processLegacySCMServiceName, []string{
		"DD_LOG_LEVEL=" + legacySCMLogLevel,
		`DD_FLEET_POLICIES_DIR=C:\stale\fleet\path`,
	}))
	require.NoError(s.T(), err)

	// The merge happens in the spawn path, so the process has to be started again for the
	// new Environment block to be read.
	_, err = host.Execute(procmgrRespawn(cli, processProcessName))
	require.NoError(s.T(), err)

	// config get asks the running process-agent for its live logger level over IPC, so this
	// holds only if the merged value reached the child's environment and took effect.
	processAgentCLI := joinWindowsPath(installRoot, "bin", "agent", "process-agent.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" config get log_level`, processAgentCLI))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Contains(ct, out, "log_level is set to: "+legacySCMLogLevel,
			"process-agent should run with DD_LOG_LEVEL merged from %s", processLegacySCMServiceName)
	}, 2*time.Minute, 5*time.Second)

	// The merge log line is written after filtering, so it lists exactly the keys that were
	// merged into the child.
	logPath := joinWindowsPath(configRoot, "logs", "dd-procmgr.log")
	out, err := host.Execute(psSelectStringLines(logPath, "legacy SCM environment variable"))
	require.NoError(s.T(), err)
	assert.Contains(s.T(), out, "DD_LOG_LEVEL")
	assert.NotContains(s.T(), out, "DD_FLEET_POLICIES_DIR",
		"the denylisted key must never be merged into the child environment")
}

// TestProcessAgentPrivilegedSpawnRejectsYamlMutation proves the on-disk processes.d YAML is
// actually validated against the embedded privileged catalog before dd-procmgr spawns
// process-agent as LocalSystem. Without that check, anyone who can write the YAML the
// installer owns could change what runs with those privileges.
//
// The catalog's three rejection reasons (command args, env, stdio) are unit-tested in
// pkg/procmgr/rust/src/platform/windows/spawn/privileged.rs. What only an installed Agent can
// show is that the file on disk reaches the validator at all, so one violation is enough here.
func (s *processProcmgrWindowsSuite) TestProcessAgentPrivilegedSpawnRejectsYamlMutation() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)

	cli := joinWindowsPath(installRoot, "bin", "agent", "dd-procmgr.exe")
	cfgPath := joinWindowsPath(installRoot, "processes.d", processProcmgrConfigFileName)

	// reload respawns a changed process only if it was running, so the validator is reached
	// only from a running process-agent.
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(procmgrCmd(cli, "describe "+processProcessName))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Equal(ct, "Running", fieldValue(out, "State"))
	}, 2*time.Minute, 5*time.Second)

	backup, err := host.Execute(psReadFileBase64(cfgPath))
	require.NoError(s.T(), err)
	backup = strings.TrimSpace(backup)

	// A rejected privileged spawn leaves the process not running, so restoring the file is
	// not enough on its own: reload restarts only processes that were running when their
	// config changed, and auto-starts only ones still in Created. Neither covers a process
	// that failed to spawn, so the restore has to start it explicitly.
	//
	// start reports an error when the process is already running, which is the usual case
	// for the deferred pass, so that error is logged rather than returned. The Running
	// assertion below is what actually proves the restore worked.
	restore := func() error {
		if _, err := host.Execute(psWriteFileBase64(cfgPath, backup)); err != nil {
			return err
		}
		if _, err := host.Execute(procmgrCmd(cli, "reload")); err != nil {
			return err
		}
		if _, err := host.Execute(procmgrCmd(cli, "start "+processProcessName)); err != nil {
			s.T().Logf("start after restoring %s reported: %v", cfgPath, err)
		}
		return nil
	}
	s.T().Cleanup(func() {
		if err := restore(); err != nil {
			s.T().Logf("failed to restore %s: %v", cfgPath, err)
		}
	})

	// The catalog allows only inherit or null for stdio, so redirecting stdout to a file is
	// the smallest edit that violates it.
	_, err = host.Execute(psReplaceInFile(cfgPath, "stdout: inherit", "stdout: C:/Windows/Temp/dd-procmgr-priv-stdout.log"))
	require.NoError(s.T(), err)
	_, err = host.Execute(procmgrCmd(cli, "reload"))
	require.NoError(s.T(), err)

	// reload first stops the running process, so anything short of Failed may just be that
	// stop in progress rather than a refused spawn.
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(procmgrCmd(cli, "describe "+processProcessName))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Equal(ct, "Failed", fieldValue(out, "State"),
			"dd-procmgr must refuse the privileged spawn when the YAML violates the catalog")
	}, 2*time.Minute, 5*time.Second)

	// Failed alone could be any spawn error. The catalog's own reason ties it to validation.
	logPath := joinWindowsPath(configRoot, "logs", "dd-procmgr.log")
	out, err := host.Execute(psSelectStringLines(logPath, "refusing privileged spawn"))
	require.NoError(s.T(), err)
	require.Contains(s.T(), out, "["+processProcessName+"] refusing privileged spawn: stdout/stderr must be inherit or null")

	require.NoError(s.T(), restore())
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(procmgrCmd(cli, "describe "+processProcessName))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Equal(ct, "Running", fieldValue(out, "State"),
			"process-agent should come back once the YAML matches the catalog again")
	}, 2*time.Minute, 5*time.Second)
}

func procmgrCmd(cli, args string) string {
	return fmt.Sprintf(`& "%s" %s`, cli, args)
}

// procmgrRespawn stops and starts a process so the spawn path runs again. `reload` only
// respawns processes whose config changed, which is not the case when only the SCM
// Environment block moved.
func procmgrRespawn(cli, process string) string {
	return fmt.Sprintf(`& "%s" stop %s; & "%s" start %s`, cli, process, cli, process)
}

// psSingleQuote renders s as a PowerShell single-quoted literal. Unlike
// escapePSSingleQuotedLiteral it does not double percent signs, because the result is used
// directly rather than passed through fmt.Sprintf.
func psSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `''`) + "'"
}

func psServiceRegistryPath(service string) string {
	return psSingleQuote(`HKLM:\SYSTEM\CurrentControlSet\Services\` + service)
}

// psSetServiceEnvironment writes the REG_MULTI_SZ Environment value that the SCM merges into
// a service's environment, which is the same value dd-procmgr reads in
// pkg/procmgr/rust/src/platform/windows/child_env.rs. Entries are KEY=VALUE.
func psSetServiceEnvironment(service string, entries []string) string {
	quoted := make([]string, 0, len(entries))
	for _, entry := range entries {
		quoted = append(quoted, psSingleQuote(entry))
	}
	return `$ErrorActionPreference='Stop'; Set-ItemProperty -LiteralPath ` + psServiceRegistryPath(service) +
		` -Name Environment -Type MultiString -Value @(` + strings.Join(quoted, ",") + `)`
}

func psClearServiceEnvironment(service string) string {
	return `$ErrorActionPreference='Stop'; Remove-ItemProperty -LiteralPath ` + psServiceRegistryPath(service) +
		` -Name Environment -ErrorAction SilentlyContinue`
}

// psSelectStringLines returns the matching lines joined by newlines, or an empty string when
// nothing matches, so callers can assert on content rather than on the exit code.
func psSelectStringLines(path, pattern string) string {
	return `$ErrorActionPreference='Stop'; (Select-String -LiteralPath ` + psSingleQuote(path) +
		` -Pattern ` + psSingleQuote(pattern) + ` | ForEach-Object { $_.Line }) -join [Environment]::NewLine`
}

func psReadFileBase64(path string) string {
	return `$ErrorActionPreference='Stop'; [Convert]::ToBase64String([IO.File]::ReadAllBytes(` + psSingleQuote(path) + `))`
}

func psWriteFileBase64(path, contents string) string {
	return `$ErrorActionPreference='Stop'; [IO.File]::WriteAllBytes(` + psSingleQuote(path) +
		`, [Convert]::FromBase64String(` + psSingleQuote(contents) + `))`
}

// psReplaceInFile fails rather than silently succeeding when old is absent, so a template
// change that renames the line breaks the test instead of making it vacuous.
func psReplaceInFile(path, old, replacement string) string {
	return `$ErrorActionPreference='Stop'; $p=` + psSingleQuote(path) +
		`; $c=[IO.File]::ReadAllText($p); $o=$c; $c=$c.Replace(` + psSingleQuote(old) + `,` + psSingleQuote(replacement) + `); ` +
		`if ($o -eq $c) { throw 'no replacement made: ' + ` + psSingleQuote(old) + ` }; ` +
		`$enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)`
}
