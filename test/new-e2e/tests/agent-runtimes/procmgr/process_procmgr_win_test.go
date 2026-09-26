// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"encoding/base64"
	"fmt"
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

	// Substring of the stale DD_FLEET_POLICIES_DIR value, distinctive enough to find in
	// process-agent's config output however YAML quotes or escapes the path.
	staleFleetPoliciesMarker = "procmgr-e2e-stale-fleet-dir"

	// processCmdPort is the process_config.cmd_port default, which the provisioned
	// datadog.yaml leaves alone. process-agent's `config` subcommands talk to whoever listens
	// on it.
	processCmdPort = 6162
)

type processProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]

	cli     string
	cfgPath string
	// installedCfgBase64 is the base64 of the processes.d YAML as installed, the baseline every
	// test starts from.
	installedCfgBase64 string
	// autoSpawnPID is the process-agent PID dd-procmgr reported after a default install, before
	// any test could start it. The cutover test requires this same PID still be Running.
	autoSpawnPID string
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

func (s *processProcmgrWindowsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
	s.cli = agentBin(installRoot, "dd-procmgr.exe")
	s.cfgPath = processesDConfig(installRoot, processProcmgrConfigFileName)

	out, err := host.Execute(psReadFileBase64(s.cfgPath))
	s.Require().NoError(err)
	s.installedCfgBase64 = strings.TrimSpace(out)

	// A host reused from an earlier run may still carry that run's mutation, and restoring
	// it before every test would then keep the host broken.
	decoded, err := base64.StdEncoding.DecodeString(s.installedCfgBase64)
	s.Require().NoError(err)
	s.Require().Contains(string(decoded), "stdout: inherit",
		"%s does not look like the installed template, so it cannot serve as the baseline", s.cfgPath)

	// This is the first half of the cutover proof, and it has to run before BeforeTest or
	// any test calls start, since either would bring up a process-agent whose automatic
	// spawn failed. The PID is kept so the cutover test can require the same process.
	s.autoSpawnPID = waitProcmgrRunning(s.T(), host, s.cli, processProcessName, 2*time.Minute)
}

// BeforeTest puts the host back to the installed baseline, so no test depends on whether an
// earlier test's cleanup succeeded. Tests run in name order on the same VM, so a failed
// cleanup would otherwise fail every test after it for reasons unrelated to what it checks.
//
// The cutover test is skipped: resetProcessAgent may stop or start process-agent, which would
// replace the auto-spawned PID SetupSuite recorded.
func (s *processProcmgrWindowsSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if testName == "TestProcessAgentCutoverSupervisedByProcmgrAndLegacySCMStopped" {
		return
	}
	s.resetProcessAgent()
}

// resetProcessAgent clears the legacy SCM Environment value, restores the processes.d YAML,
// and waits for process-agent to be Running.
//
// It starts process-agent only from Failed or Stopped, which are the states tests leave it
// in. From any other state dd-procmgr is still acting on its own, so it waits.
func (s *processProcmgrWindowsSuite) resetProcessAgent() {
	host := s.Env().RemoteHost

	out, err := host.Execute(psClearServiceEnvironment(processLegacySCMServiceName))
	s.Require().NoError(err)
	if strings.TrimSpace(out) == "Removed" {
		// The value is read only at spawn, so a process started while it was set keeps it
		// until it is spawned again.
		if _, err := host.Execute(procmgrCmd(s.cli, "stop "+processProcessName)); err != nil {
			s.T().Logf("stop after clearing the %s Environment value reported: %v", processLegacySCMServiceName, err)
		}
	}

	// Every step is repeated on each attempt: dd-procmgr may still be starting on the first
	// test, and writing identical bytes and reloading an unchanged config are no-ops.
	s.Require().EventuallyWithT(func(ct *assert.CollectT) {
		if _, err := host.Execute(psWriteFileBase64(s.cfgPath, s.installedCfgBase64)); !assert.NoError(ct, err) {
			return
		}
		if _, err := host.Execute(procmgrCmd(s.cli, "reload")); !assert.NoError(ct, err) {
			return
		}
		out, err := host.Execute(procmgrCmd(s.cli, "describe "+processProcessName))
		if !assert.NoError(ct, err) {
			return
		}
		state := fieldValue(out, "State")
		if state == "Failed" || state == "Stopped" {
			_, err := host.Execute(procmgrCmd(s.cli, "start "+processProcessName))
			assert.NoError(ct, err)
		}
		assert.Equal(ct, "Running", state, "process-agent should be Running before each test: %s", out)
	}, 3*time.Minute, 5*time.Second)
}

// TestProcessAgentCutoverSupervisedByProcmgrAndLegacySCMStopped is the end-to-end proof of the
// Windows cutover: on a default install dd-procmgr brings process-agent up on its own, and
// the core Agent leaves the legacy SCM service alone. Both halves have to hold at once.
// Either one alone is a bug: only the first means two process-agents, only the second means
// none at all.
//
// SetupSuite records the auto-spawned PID before any test can start process-agent. This test
// requires that same PID still be Running (same process), and that the legacy service stayed
// down. The method name starts with Cutover so it runs first in lexical order, before tests
// that respawn process-agent.
func (s *processProcmgrWindowsSuite) TestProcessAgentCutoverSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	// Unlike PAR, process-agent ships with every install, so a missing binary is a failure
	// rather than a reason to skip.
	requireHostPath(s.T(), host, agentBin(installRoot, "process-agent.exe"),
		"process-agent.exe should be installed at %s")
	requireHostPath(s.T(), host, processesDConfig(installRoot, processProcmgrConfigFileName),
		"fleet process-agent processes.d config should exist at %s")

	requireProcmgrRunningPID(s.T(), host, s.cli, processProcessName, s.autoSpawnPID, 2*time.Minute)

	// Anything short of Stopped, StartPending in particular, can be the SCM on its way to a
	// second process-agent, so only Stopped or Absent passes.
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(
			`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`,
			processLegacySCMServiceName,
		))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Contains(ct, []string{"Stopped", "Absent"}, strings.TrimSpace(out),
			"%s Windows service must stay down when process-agent is managed by dd-procmgr", processLegacySCMServiceName)
	}, 30*time.Second, 3*time.Second)
}

// TestProcessAgentInheritsFilteredLegacyScmEnvironment covers the environment hand-off that
// makes the cutover transparent: process-agent used to run as an SCM service and pick up the
// Environment block configured on it, so dd-procmgr merges that same block when it spawns the
// process. The denylist matters as much as the merge. process-agent reads its fleet policy
// directory from the registry only when fleet_policies_dir is not already configured, so a
// stale DD_FLEET_POLICIES_DIR inherited from the legacy service would silently win and point
// process-agent at the wrong policies.
func (s *processProcmgrWindowsSuite) TestProcessAgentInheritsFilteredLegacyScmEnvironment() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)

	cli := s.cli

	// BeforeTest repairs the host for the next test regardless. Failing here still points at
	// the test that left it broken.
	s.T().Cleanup(func() {
		if _, err := host.Execute(psClearServiceEnvironment(processLegacySCMServiceName)); err != nil {
			s.T().Errorf("failed to clear the %s Environment value: %v", processLegacySCMServiceName, err)
		}
		if _, err := host.Execute(procmgrRespawn(cli, processProcessName)); err != nil {
			s.T().Errorf("failed to respawn %s: %v", processProcessName, err)
		}
	})

	// The merge log line is written after filtering, so it lists exactly the keys handed to
	// the child. The log is shared by every test, so only lines added by this respawn count.
	logPath := joinWindowsPath(configRoot, "logs", "dd-procmgr.log")
	mergeLinePrefix := "[" + processProcessName + "] applying "
	mergeLines := func() ([]string, error) {
		out, err := host.Execute(psSelectStringLines(logPath, "legacy SCM environment variable"))
		if err != nil {
			return nil, err
		}
		var lines []string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, mergeLinePrefix) {
				lines = append(lines, strings.TrimSpace(line))
			}
		}
		return lines, nil
	}
	linesBefore, err := mergeLines()
	require.NoError(s.T(), err)

	_, err = host.Execute(psSetServiceEnvironment(processLegacySCMServiceName, []string{
		"DD_LOG_LEVEL=" + legacySCMLogLevel,
		`DD_FLEET_POLICIES_DIR=C:\` + staleFleetPoliciesMarker,
	}))
	require.NoError(s.T(), err)

	// The merge happens in the spawn path, so the process has to be started again for the
	// new Environment block to be read.
	_, err = host.Execute(procmgrRespawn(cli, processProcessName))
	require.NoError(s.T(), err)

	// Checked before the live level so a failure says which half broke: dd-procmgr never
	// merging the block, or process-agent not honoring a value it was handed.
	var merged string
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		lines, err := mergeLines()
		if !assert.NoError(ct, err) {
			return
		}
		if assert.Greater(ct, len(lines), len(linesBefore),
			"dd-procmgr should log merging the %s Environment block when respawning %s",
			processLegacySCMServiceName, processProcessName) {
			merged = lines[len(lines)-1]
		}
	}, 30*time.Second, 3*time.Second)
	require.Contains(s.T(), merged, "DD_LOG_LEVEL")
	require.NotContains(s.T(), merged, "DD_FLEET_POLICIES_DIR",
		"the denylisted key must never be merged into the child environment")

	// config get asks the running process-agent for its live logger level over IPC, so this
	// holds only if the merged value reached the child's environment and took effect.
	processAgentCLI := agentBin(installRoot, "process-agent.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		// A process-agent that outlived the stop keeps the port, so config get would read
		// its level instead of the respawned one's.
		desc, err := host.Execute(procmgrCmd(cli, "describe "+processProcessName))
		if !assert.NoError(ct, err) {
			return
		}
		owner, err := host.Execute(psListeningPortOwner(processCmdPort))
		if !assert.NoError(ct, err) {
			return
		}
		supervised := fieldValue(desc, "PID")
		if !assert.Equal(ct, supervised, strings.TrimSpace(owner),
			"port %d should belong to the process-agent dd-procmgr supervises (PID %s): %s",
			processCmdPort, supervised, desc) {
			return
		}

		out, err := host.Execute(fmt.Sprintf(`& "%s" config get log_level`, processAgentCLI))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Contains(ct, out, "log_level is set to: "+legacySCMLogLevel,
			"process-agent should run with DD_LOG_LEVEL merged from %s", processLegacySCMServiceName)
	}, 2*time.Minute, 5*time.Second)

	// The same running process-agent, which the check above proved reads its merged
	// environment, must not have picked up the denylisted value. /config/all renders
	// defaults too, so fleet_policies_dir is always listed and its absence cannot make this
	// pass.
	out, err := host.Execute(fmt.Sprintf(`& "%s" config --all`, processAgentCLI))
	require.NoError(s.T(), err)
	require.Contains(s.T(), out, "fleet_policies_dir")
	assert.NotContains(s.T(), out, staleFleetPoliciesMarker,
		"the denylisted DD_FLEET_POLICIES_DIR must not reach process-agent's config")
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
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)

	// reload respawns a changed process only if it was running, so the validator is reached
	// only from a running process-agent. BeforeTest leaves it Running.
	cli := s.cli
	cfgPath := s.cfgPath

	// A rejected privileged spawn leaves the process not running, so restoring the file is
	// not enough on its own: reload restarts only processes that were running when their
	// config changed, and auto-starts only ones still in Created. Neither covers a process
	// that failed to spawn, so the restore has to start it explicitly.
	//
	// start reports an error when the process is already running, which is the usual case
	// for the deferred pass, so that error is logged rather than returned. The Running
	// assertion below is what actually proves the restore worked.
	restore := func() error {
		if _, err := host.Execute(psWriteFileBase64(cfgPath, s.installedCfgBase64)); err != nil {
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
			s.T().Errorf("failed to restore %s: %v", cfgPath, err)
		}
	})

	// The log outlives the test, so a run reusing this host already has the rejection line.
	// Only a line written after this point proves this mutation reached the validator.
	logPath := joinWindowsPath(configRoot, "logs", "dd-procmgr.log")
	rejection := "[" + processProcessName + "] refusing privileged spawn: stdout/stderr must be inherit or null"
	countRejections := func() int {
		out, err := host.Execute(psSelectStringLines(logPath, "refusing privileged spawn"))
		require.NoError(s.T(), err)
		return strings.Count(out, rejection)
	}
	rejectionsBefore := countRejections()

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
	require.Greater(s.T(), countRejections(), rejectionsBefore,
		"dd-procmgr.log should gain a %q line for this mutation", rejection)

	require.NoError(s.T(), restore())
	_ = waitProcmgrRunning(s.T(), host, cli, processProcessName, 2*time.Minute)
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

// psClearServiceEnvironment prints Removed when there was a value to remove, and nothing
// otherwise.
func psClearServiceEnvironment(service string) string {
	key := psServiceRegistryPath(service)
	return `$ErrorActionPreference='Stop'; if ($null -ne (Get-ItemProperty -LiteralPath ` + key +
		` -Name Environment -ErrorAction SilentlyContinue)) { Remove-ItemProperty -LiteralPath ` + key +
		` -Name Environment; 'Removed' }`
}

// psSelectStringLines returns the matching lines joined by newlines, or an empty string when
// nothing matches, so callers can assert on content rather than on the exit code.
func psSelectStringLines(path, pattern string) string {
	return `$ErrorActionPreference='Stop'; (Select-String -LiteralPath ` + psSingleQuote(path) +
		` -Pattern ` + psSingleQuote(pattern) + ` | ForEach-Object { $_.Line }) -join [Environment]::NewLine`
}

// psListeningPortOwner prints the PIDs listening on port, comma-separated, or nothing when
// the port is free.
func psListeningPortOwner(port int) string {
	return fmt.Sprintf(`$ErrorActionPreference='Stop'; (Get-NetTCPConnection -LocalPort %d -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique) -join ','`, port)
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
