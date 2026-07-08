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

	processBin := filepath.Join(installRoot, "bin", "agent", "process-agent.exe")
	_, err = host.Execute(fmt.Sprintf(`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`, processBin))
	require.NoError(s.T(), err, "process-agent.exe should be installed at %s", processBin)

	cfg := filepath.Join(installRoot, "processes.d", processAgentProcmgrConfigFile)
	_, err = host.Execute(fmt.Sprintf(`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`, cfg))
	require.NoError(s.T(), err, "fleet process-agent processes.d config should exist at %s", cfg)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processAgentProcessName))
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
	}, 120*time.Second, 3*time.Second)

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		// Verify privilege level: legacy `datadog-process-agent` runs as LocalSystem.
		// When `process-agent` is started by dd-procmgr, it must preserve that behavior.
		ownerOut, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''" | Select-Object -First 1; if ($null -eq $p) { exit 1 }; $o = Invoke-CimMethod -InputObject $p -MethodName GetOwner; if ($o.ReturnValue -ne 0) { exit $o.ReturnValue }; "$($o.Domain)/$($o.User)"'`)
		assert.NoError(ct, err)
		assert.Contains(ct, ownerOut, "NT AUTHORITY/SYSTEM")
	}, 120*time.Second, 3*time.Second)

	expectedCfg := filepath.Join(configRoot, "datadog.yaml")
	expectedSysprobe := filepath.Join(configRoot, "system-probe.yaml")
	expectedPid := filepath.Join(installRoot, "run", "process-agent.pid")

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		cmdLine, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''" | Select-Object -First 1; if ($null -eq $p) { exit 1 }; $p.CommandLine'`)
		assert.NoError(ct, err)

		// Normalize slashes and casing to avoid false negatives due to path formatting differences.
		cmdNorm := strings.ToLower(strings.ReplaceAll(cmdLine, "/", "\\"))
		processBinNorm := strings.ToLower(strings.ReplaceAll(processBin, "/", "\\"))
		expectedCfgNorm := strings.ToLower(strings.ReplaceAll(expectedCfg, "/", "\\"))
		expectedSysprobeNorm := strings.ToLower(strings.ReplaceAll(expectedSysprobe, "/", "\\"))
		expectedPidNorm := strings.ToLower(strings.ReplaceAll(expectedPid, "/", "\\"))

		// Assert the command line matches our embedded privileged catalog spec
		// (exact flags + values).
		assert.Contains(ct, cmdNorm, processBinNorm)

		idxCfgFlag := strings.Index(cmdNorm, "--cfgpath")
		idxCfgVal := strings.Index(cmdNorm, expectedCfgNorm)
		idxSysFlag := strings.Index(cmdNorm, "--sysprobe-config")
		idxSysVal := strings.Index(cmdNorm, expectedSysprobeNorm)
		idxPidFlag := strings.Index(cmdNorm, "--pid")
		idxPidVal := strings.Index(cmdNorm, expectedPidNorm)

		assert.GreaterOrEqual(ct, idxCfgFlag, 0)
		assert.GreaterOrEqual(ct, idxCfgVal, 0)
		assert.GreaterOrEqual(ct, idxSysFlag, 0)
		assert.GreaterOrEqual(ct, idxSysVal, 0)
		assert.GreaterOrEqual(ct, idxPidFlag, 0)
		assert.GreaterOrEqual(ct, idxPidVal, 0)

		// Order check: --cfgpath <cfg> ... --sysprobe-config <sysprobe> ... --pid <pid>
		assert.Less(ct, idxCfgFlag, idxCfgVal)
		assert.Less(ct, idxCfgVal, idxSysFlag)
		assert.Less(ct, idxSysFlag, idxSysVal)
		assert.Less(ct, idxSysVal, idxPidFlag)
		assert.Less(ct, idxPidFlag, idxPidVal)
	}, 120*time.Second, 3*time.Second)

	_, err = host.Execute(
		fmt.Sprintf(`powershell -NoProfile -Command "$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 0 }; if ($s.Status -eq 'Running') { exit 1 }; exit 0"`, processAgentLegacySCMServiceName))
	require.NoError(s.T(), err, "%s Windows service must not be Running when process-agent is managed by dd-procmgr", processAgentLegacySCMServiceName)
}

func (s *processProcmgrWindowsSuite) TestProcessAgentPrivilegedSpawnCatalogEnforcedAgainstYamlMutation() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	cfgPath := filepath.Join(installRoot, "processes.d", processAgentProcmgrConfigFile)

	expectedCfg := filepath.Join(configRoot, "datadog.yaml")
	expectedCfgSlash := strings.ReplaceAll(expectedCfg, `\`, `/`)
	tamperedCfgSlash := expectedCfgSlash + ".tampered"

	// Backup the original YAML so we can restore it at the end of the test.
	backupPS := fmt.Sprintf(`powershell -NoProfile -Command "$p = '%s'; $c = Get-Content -Raw -LiteralPath $p; [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($c))"`,
		escapePSSingleQuotedLiteral(cfgPath))
	backupB64, err := host.Execute(backupPS)
	require.NoError(s.T(), err)
	backupB64 = strings.TrimSpace(backupB64)

	// Restore config at the end (best-effort; the assertions below are the real signal).
	restore := func() {
		restorePS := fmt.Sprintf(`powershell -NoProfile -Command "$p = '%s'; $b = [Convert]::FromBase64String('%s'); [IO.File]::WriteAllBytes($p, $b)"`,
			escapePSSingleQuotedLiteral(cfgPath), backupB64)
		_, err := host.Execute(restorePS)
		require.NoError(s.T(), err)
		require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))
	}
	defer restore()

	// Mutate the YAML: change --cfgpath value so it no longer matches the embedded privileged catalog.
	//
	// Stage 1: Mutate privileged *command args* (the --cfgpath value) so dd-procmgr's embedded privileged
	// catalog validation rejects the privileged spawn request.
	mutatePS := fmt.Sprintf(`powershell -NoProfile -Command "$p = '%s'; $old = '%s'; $new = '%s'; $c = Get-Content -Raw -LiteralPath $p; $o = $c; $c = $c.Replace($old, $new); if ($o -eq $c) { exit 1 }; Set-Content -LiteralPath $p -Value $c -Encoding utf8"`,
		escapePSSingleQuotedLiteral(cfgPath), escapePSSingleQuotedLiteral(expectedCfgSlash), escapePSSingleQuotedLiteral(tamperedCfgSlash))
	_, err = host.Execute(mutatePS)
	require.NoError(s.T(), err)

	// If the privileged catalog validation is enforced, dd-procmgr should refuse to spawn
	// the privileged process with the mutated args, so process-agent should not come back.
	require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''"; if ($null -eq $p) { exit 0 } else { exit 1 }'`)
		assert.NoError(ct, err)

		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processAgentProcessName))
		assert.NoError(ct, err)
		assertField(ct, out, "State", "Failed")
	}, 120*time.Second, 3*time.Second)

	// Restore original YAML and ensure dd-procmgr can spawn the process-agent again.
	restore()
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''"; if ($null -eq $p) { exit 1 } else { exit 0 }'`)
		assert.NoError(ct, err)
	}, 120*time.Second, 3*time.Second)

	// Stage 2: Mutate privileged *env* (DD_FLEET_POLICIES_DIR) so it violates the non-empty rule.
	// This should make dd-procmgr refuse the privileged spawn request.
	envMutatePS := fmt.Sprintf(`powershell -NoProfile -Command "$p = '%s'; $c = Get-Content -Raw -LiteralPath $p; $o = $c; $c = $c -replace '(?m)^(\s*DD_FLEET_POLICIES_DIR:\s*).*$','$1""'; if ($o -eq $c) { exit 1 }; Set-Content -LiteralPath $p -Value $c -Encoding utf8"`,
		escapePSSingleQuotedLiteral(cfgPath))
	_, err = host.Execute(envMutatePS)
	require.NoError(s.T(), err)

	require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''"; if ($null -eq $p) { exit 0 } else { exit 1 }'`)
		assert.NoError(ct, err)

		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processAgentProcessName))
		assert.NoError(ct, err)
		assertField(ct, out, "State", "Failed")
	}, 120*time.Second, 3*time.Second)

	restore()
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''"; if ($null -eq $p) { exit 1 } else { exit 0 }'`)
		assert.NoError(ct, err)
	}, 120*time.Second, 3*time.Second)

	// Stage 3: Mutate privileged *stdio* (stdout) so it violates the inherit/null allowlist.
	// This should make dd-procmgr refuse the privileged spawn request.
	stdoutFile := `C:/Windows/Temp/dd-procmgr-priv-stdout.log`
	stdioMutatePS := fmt.Sprintf(`powershell -NoProfile -Command "$p = '%s'; $c = Get-Content -Raw -LiteralPath $p; $o = $c; $c = $c.Replace('stdout: inherit', 'stdout: %s'); if ($o -eq $c) { exit 1 }; Set-Content -LiteralPath $p -Value $c -Encoding utf8"`,
		escapePSSingleQuotedLiteral(cfgPath), stdoutFile)
	_, err = host.Execute(stdioMutatePS)
	require.NoError(s.T(), err)

	require.NoError(s.T(), windowscommon.RestartService(host, "datadogagent"))

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''"; if ($null -eq $p) { exit 0 } else { exit 1 }'`)
		assert.NoError(ct, err)

		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processAgentProcessName))
		assert.NoError(ct, err)
		assertField(ct, out, "State", "Failed")
	}, 120*time.Second, 3*time.Second)

	restore()
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		_, err := host.Execute(`powershell -NoProfile -Command '$p = Get-CimInstance Win32_Process -Filter "Name=''process-agent.exe''"; if ($null -eq $p) { exit 1 } else { exit 0 }'`)
		assert.NoError(ct, err)
	}, 120*time.Second, 3*time.Second)
}
