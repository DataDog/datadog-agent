// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
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
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	processAgentProcessName          = "datadog-agent-process"
	processAgentLegacySCMServiceName = "datadog-process-agent"
	processAgentProcmgrConfigFile    = "datadog-agent-process.yaml"
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

func (s *processProcmgrWindowsSuite) TestProcessAgentSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)

	processBin := joinWindowsPath(installRoot, "bin", "agent", "process-agent.exe")
	_, err = host.Execute(psLiteralPathExists(processBin))
	require.NoError(s.T(), err, "process-agent.exe should be installed at %s", processBin)

	cfg := joinWindowsPath(installRoot, "processes.d", processAgentProcmgrConfigFile)
	_, err = host.Execute(psLiteralPathExists(cfg))
	require.NoError(s.T(), err, "fleet process-agent processes.d config should exist at %s", cfg)

	cli := joinWindowsPath(installRoot, "bin", "agent", "dd-procmgr.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processAgentProcessName))
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
	}, 120*time.Second, 3*time.Second)

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		// Verify privilege level: legacy `datadog-process-agent` runs as LocalSystem.
		// When `process-agent` is started by dd-procmgr, it must preserve that behavior.
		ownerOut, err := windowsProcessOwnerByName(host, "process-agent.exe")
		assert.NoError(ct, err)
		assert.Contains(ct, ownerOut, "NT AUTHORITY/SYSTEM")
	}, 120*time.Second, 3*time.Second)

	expectedCfg := joinWindowsPath(configRoot, "datadog.yaml")

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		cmdLine, err := host.Execute(psRemote(`$p = Get-CimInstance Win32_Process -Filter "Name='process-agent.exe'" | Select-Object -First 1; if ($null -eq $p) { exit 1 }; $p.CommandLine`))
		assert.NoError(ct, err)

		cmdNorm := normalizeWindowsPathForCompare(cmdLine)
		processBinNorm := normalizeWindowsPathForCompare(processBin)
		expectedCfgNorm := normalizeWindowsPathForCompare(expectedCfg)

		// Assert the command line matches legacy SCM registration and the embedded privileged catalog.
		assert.Contains(ct, cmdNorm, processBinNorm)

		idxCfgFlag := strings.Index(cmdNorm, "--cfgpath")
		idxCfgVal := strings.Index(cmdNorm, expectedCfgNorm)

		assert.GreaterOrEqual(ct, idxCfgFlag, 0)
		assert.GreaterOrEqual(ct, idxCfgVal, 0)
		assert.Less(ct, idxCfgFlag, idxCfgVal)
	}, 120*time.Second, 3*time.Second)

	_, err = host.Execute(psLegacySCMServiceMustNotBeRunning(processAgentLegacySCMServiceName))
	require.NoError(s.T(), err, "%s Windows service must not be Running when process-agent is managed by dd-procmgr", processAgentLegacySCMServiceName)
}

func (s *processProcmgrWindowsSuite) TestProcessAgentPrivilegedSpawnCatalogEnforcedAgainstYamlMutation() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)

	cli := joinWindowsPath(installRoot, "bin", "agent", "dd-procmgr.exe")
	cfgPath := joinWindowsPath(installRoot, "processes.d", processAgentProcmgrConfigFile)

	expectedCfgSlash := joinWindowsPath(configRoot, "datadog.yaml")
	tamperedCfgSlash := expectedCfgSlash + ".tampered"

	// Backup the original YAML so we can restore it at the end of the test.
	backupPS := psRemote(`$p = '%s'; $c = Get-Content -Raw -LiteralPath $p; [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($c))`, cfgPath)
	backupB64, err := host.Execute(backupPS)
	require.NoError(s.T(), err)
	backupB64 = strings.TrimSpace(backupB64)

	// Restore config at the end (best-effort; the assertions below are the real signal).
	restore := func() {
		restorePS := psRemote(`$p = '%s'; $b = [Convert]::FromBase64String('%s'); [IO.File]::WriteAllBytes($p, $b)`, cfgPath, backupB64)
		_, err := host.Execute(restorePS)
		require.NoError(s.T(), err)
		require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))
	}
	defer restore()

	// Mutate the YAML: change --cfgpath value so it no longer matches the embedded privileged catalog.
	//
	// Stage 1: Mutate privileged *command args* (the --cfgpath value) so dd-procmgr's embedded privileged
	// catalog validation rejects the privileged spawn request.
	_, err = host.Execute(replaceProcessesDYaml(cfgPath, expectedCfgSlash, tamperedCfgSlash))
	require.NoError(s.T(), err)

	require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		assertPrivilegedCatalogRejection(ct, host, cli, processAgentProcessName)
	}, 120*time.Second, 3*time.Second)

	// Restore original YAML and ensure dd-procmgr can spawn the process-agent again.
	restore()
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(psProcessExistsByName("process-agent.exe"))
		assert.NoError(ct, err)
	}, 120*time.Second, 3*time.Second)

	// Stage 2: Inject a disallowed env var into privileged process YAML. The catalog allows
	// no config-supplied env vars (fleet policy dir comes from the registry at runtime).
	_, err = host.Execute(psRemote(
		`$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $o=$c; $nl=[Environment]::NewLine; $repl='start_limit_burst: 5'+$nl+'env:'+$nl+'  DD_MALICIOUS_VAR: tampered'; $c=$c.Replace('start_limit_burst: 5',$repl); if ($o -eq $c) { exit 1 }; $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)`,
		cfgPath,
	))
	require.NoError(s.T(), err)

	require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		assertPrivilegedCatalogRejection(ct, host, cli, processAgentProcessName)
	}, 120*time.Second, 3*time.Second)

	restore()
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(psProcessExistsByName("process-agent.exe"))
		assert.NoError(ct, err)
	}, 120*time.Second, 3*time.Second)

	// Stage 3: Mutate privileged *stdio* (stdout) so it violates the inherit/null allowlist.
	// This should make dd-procmgr refuse the privileged spawn request.
	stdoutFile := `C:/Windows/Temp/dd-procmgr-priv-stdout.log`
	_, err = host.Execute(replaceProcessesDYaml(cfgPath, "stdout: inherit", "stdout: "+stdoutFile))
	require.NoError(s.T(), err)

	require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		assertPrivilegedCatalogRejection(ct, host, cli, processAgentProcessName)
	}, 120*time.Second, 3*time.Second)

	restore()
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(psProcessExistsByName("process-agent.exe"))
		assert.NoError(ct, err)
	}, 120*time.Second, 3*time.Second)
}
