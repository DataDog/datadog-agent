// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	winDaemonBin = `C:\Program Files\Datadog\Datadog Agent\bin\agent\dd-procmgrd.exe`
	winCLIBin    = `C:\Program Files\Datadog\Datadog Agent\bin\agent\dd-procmgr.exe`
	// Must match dd-procmgrd default on Windows: install root + processes.d
	// (see pkg/procmgr/rust/src/platform/windows.rs default_config_dir).
	winConfigDir = `C:/Program Files/Datadog/Datadog Agent/processes.d`

	winSleepCommand = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`

	winTestProcessConfig = `command: C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe
args:
  - "-NoProfile"
  - "-NonInteractive"
  - "-Command"
  - "Start-Sleep -Seconds 3600"
env:
  SystemRoot: C:\Windows
  PATH: C:\Windows\System32;C:\Windows
auto_start: true
restart: always
description: E2E test process
`

	winMissingBinaryConfig = `command: C:\nonexistent\binary.exe
condition_path_exists: C:\nonexistent\binary.exe
auto_start: true
restart: never
description: should not start
`

	// Same description line as fleet/embedded DDOT processes.d template so reload tests can
	// mutate it and assert describe output.
	windowsDDOTDescOriginalLine = "description: Datadog Distribution of OpenTelemetry Collector"
	windowsDDOTDescE2ELine      = "description: E2E-reload-after-yaml"

	windowsADPDescOriginalLine = "description: Datadog Agent Data Plane"
	windowsADPDescE2ELine      = "description: E2E-reload-after-yaml"

	adpProcessName = "datadog-agent-data-plane"
)

// psRemote formats a PowerShell script for RemoteHost.Execute on Windows.
// String args are escaped for single-quoted literals: ” for quotes, %% for fmt.Sprintf.
// $ and $env:... in args are safe because they are only embedded inside '...', not expanded.
func psRemote(format string, args ...any) string {
	for i, a := range args {
		if s, ok := a.(string); ok {
			args[i] = escapePSSingleQuotedLiteral(s)
		}
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func escapePSSingleQuotedLiteral(s string) string {
	s = strings.ReplaceAll(s, `%`, `%%`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return s
}

// toWindowsSlashPath normalizes a Windows path to forward slashes. The e2e runner is
// Linux/macOS where filepath.ToSlash is a no-op on backslashes.
func toWindowsSlashPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, `/`)
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

// joinWindowsPath joins a Windows install/config path with relative segments using
// forward slashes. The e2e runner is Linux; installPath from the registry uses
// backslashes, so filepath.Join would produce mixed separators.
func joinWindowsPath(base string, elems ...string) string {
	p := strings.TrimRight(toWindowsSlashPath(base), `/`)
	for _, e := range elems {
		p += "/" + e
	}
	return p
}

// ensureWindowsDirPS returns a script that creates a directory on the remote host.
// Use -Path (not -LiteralPath): Windows PowerShell 5.1 New-Item does not support -LiteralPath.
func ensureWindowsDirPS(dir string) string {
	return psRemote(`New-Item -ItemType Directory -Force -Path '%s' | Out-Null`, dir)
}

var winPlatform = platformConfig{
	daemonBin:         winDaemonBin,
	cliBin:            winCLIBin,
	configDir:         winConfigDir,
	sleepCommand:      winSleepCommand,
	testProcessYAML:   winTestProcessConfig,
	missingBinaryYAML: winMissingBinaryConfig,
	checkBinCmd: func(path string) string {
		return psRemote(`if (Test-Path -LiteralPath '%s') { exit 0 } else { exit 1 }`, path)
	},
	checkSvcRunning:  `powershell -Command "(Get-Service dd-procmgr-service).Status"`,
	svcRunningOutput: "Running",
	cliCmd: func(args string) string {
		return fmt.Sprintf(`& "%s" %s`, winCLIBin, args)
	},
	killPIDCmd: func(pid uint32) string {
		return fmt.Sprintf(`powershell -NoProfile -Command "Stop-Process -Id %d -Force"`, pid)
	},
}

type procmgrWindowsSuite struct {
	baseProcmgrSuite
	hasDDOT bool
	hasADP  bool
}

func TestProcmgrSmokeWindowsSuite(t *testing.T) {
	t.Parallel()
	s := &procmgrWindowsSuite{}
	s.platform = winPlatform
	e2e.Run(t, s, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentparams.WithFile(winConfigDir+"/test-sleep.yaml", winTestProcessConfig, true),
					agentparams.WithFile(winConfigDir+"/missing-binary.yaml", winMissingBinaryConfig, true),
				),
			),
		),
	))
}

func (s *procmgrWindowsSuite) SetupSuite() {
	s.baseProcmgrSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// dd-procmgr-service is DEMAND_START; the agent starts it as a dependent
	// service, but on a fresh install the timing is unpredictable. Ensure the
	// service is running before the tests begin.
	s.Env().RemoteHost.MustExecute(`powershell -Command "Start-Service dd-procmgr-service"`)

	if s.hasCLI {
		require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
			out := s.Env().RemoteHost.MustExecuteOn(t,
				`powershell -Command "(Get-Service dd-procmgr-service).Status"`)
			assert.Equal(t, "Running", strings.TrimSpace(out))
		}, 60*time.Second, 2*time.Second)
	}

	s.tryInstallWindowsDDOTForProcmgr()
	s.hasADP = s.trySetupWindowsADPForProcmgr()
}

// tryInstallWindowsDDOTForProcmgr mirrors the Linux procmgr DDOT setup: copy embedded otel-agent
// into ext/ddot, bootstrap otel-config.yaml, enable otelcollector in datadog.yaml, write
// processes.d/datadog-agent-ddot.yaml, and reload. Skips (hasDDOT=false) if embedded otel-agent
// or otel-config bootstrap is unavailable on this image.
func (s *procmgrWindowsSuite) tryInstallWindowsDDOTForProcmgr() {
	s.T().Helper()
	s.hasDDOT = false
	host := s.Env().RemoteHost

	installPath, err := windowsagent.GetInstallPathFromRegistry(host)
	if err != nil {
		s.T().Logf("windows ddot procmgr: InstallPath registry: %v", err)
		return
	}
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	if err != nil {
		s.T().Logf("windows ddot procmgr: ConfigRoot registry: %v", err)
		return
	}

	embeddedOtel := filepath.Join(installPath, "embedded", "bin", "otel-agent.exe")
	if _, err := host.Execute(s.platform.checkBinCmd(embeddedOtel)); err != nil {
		s.T().Logf("windows ddot procmgr: no embedded otel-agent at %s", embeddedOtel)
		return
	}

	destExe := filepath.Join(installPath, "ext", "ddot", "embedded", "bin", "otel-agent.exe")
	destDir := filepath.Dir(destExe)
	copyPS := psRemote(
		`$ErrorActionPreference='Stop'; New-Item -ItemType Directory -Force -Path '%s' | Out-Null; Copy-Item -LiteralPath '%s' -Destination '%s' -Force`,
		destDir, embeddedOtel, destExe,
	)
	if _, err := host.Execute(copyPS); err != nil {
		s.T().Logf("windows ddot procmgr: copy otel-agent failed: %v", err)
		return
	}

	exExample := filepath.Join(configRoot, "otel-config.yaml.example")
	exOut := filepath.Join(configRoot, "otel-config.yaml")
	otelPS := psRemote(
		`$ErrorActionPreference='Stop'; $ex='%s'; $out='%s'; if (Test-Path -LiteralPath $ex) { $c = Get-Content -Raw -LiteralPath $ex; $c = $c -replace '\$\{env:DD_API_KEY\}','aaaaaaaaaaaaaaaa'; $c = $c -replace '\$\{env:DD_SITE\}','datadoghq.com'; Set-Content -LiteralPath $out -Value $c -Encoding utf8 } elseif (-not (Test-Path -LiteralPath $out)) { throw 'missing otel-config' }`,
		exExample, exOut,
	)
	if _, err := host.Execute(otelPS); err != nil {
		s.T().Logf("windows ddot procmgr: otel-config bootstrap failed: %v", err)
		return
	}

	fleetPolicies := filepath.Join(configRoot, "Installer", "managed", "datadog-agent", "stable")
	host.MustExecute(ensureWindowsDirPS(fleetPolicies))

	datadogYAML := filepath.Join(configRoot, "datadog.yaml")
	appendOtel := "\notelcollector:\n  enabled: true\n"
	b64AppendOtel := base64.StdEncoding.EncodeToString([]byte(appendOtel))
	host.MustExecute(psRemote(
		`$dy='%s'; if (-not (Test-Path -LiteralPath $dy)) { exit 0 }; if (-not (Select-String -LiteralPath $dy -Pattern 'otelcollector:' -Quiet)) { $a=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); Add-Content -LiteralPath $dy -Value $a -Encoding utf8 }`,
		datadogYAML, b64AppendOtel,
	))

	yamlPath := filepath.Join(installPath, "processes.d", "datadog-agent-ddot.yaml")
	yamlBody := windowsDDOTProcmgrYAMLContent(installPath, configRoot, fleetPolicies)
	b64 := base64.StdEncoding.EncodeToString([]byte(yamlBody))
	if _, err := host.Execute(psRemote(
		`$ErrorActionPreference='Stop'; $b=[Convert]::FromBase64String('%s'); [IO.File]::WriteAllBytes('%s', $b)`,
		b64, yamlPath,
	)); err != nil {
		s.T().Logf("windows ddot procmgr: write processes.d yaml failed: %v", err)
		return
	}

	if _, err := host.Execute(s.platform.cliCmd("reload")); err != nil {
		s.T().Logf("windows ddot procmgr: initial reload failed: %v", err)
		return
	}

	s.hasDDOT = true
}

func windowsDDOTProcmgrYAMLContent(installPath, configRoot, fleetPolicies string) string {
	toSlash := func(p string) string {
		return filepath.ToSlash(p)
	}
	exe := toSlash(filepath.Join(installPath, "ext", "ddot", "embedded", "bin", "otel-agent.exe"))
	otelCfg := toSlash(filepath.Join(configRoot, "otel-config.yaml"))
	ddCfg := toSlash(filepath.Join(configRoot, "datadog.yaml"))
	fleet := toSlash(fleetPolicies)
	return fmt.Sprintf(`%s
command: %s
args:
  - run
  - --sync-delay
  - 90s
  - --config
  - %s
  - --core-config
  - %s
auto_start: true
condition_path_exists: %s
restart: on-failure
restart_sec: 2
start_limit_interval_sec: 10
start_limit_burst: 5
env:
  DD_OTELCOLLECTOR_ENABLED: "true"
  DD_FLEET_POLICIES_DIR: %s
  DD_OTELCOLLECTOR_INSTALLATION_METHOD: bare-metal
stdout: inherit
stderr: inherit
`, windowsDDOTDescOriginalLine, exe, otelCfg, ddCfg, exe, fleet)
}

func (s *procmgrWindowsSuite) requireDDOTWindows() {
	s.T().Helper()
	if !s.hasDDOT {
		s.T().Skip("windows ddot procmgr: embedded otel-agent or otel-config bootstrap not available on this image")
	}
	s.requireCLI()
}

func (s *procmgrWindowsSuite) waitWindowsDDOTRunning(timeout time.Duration) string {
	s.T().Helper()
	var pid string
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.platform.cliCmd("describe datadog-agent-ddot"))
		assertField(ct, out, "State", "Running")
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(ct, p) || !assert.NotEqual(ct, "-", p) {
			return
		}
		cmd := fieldValue(out, "Command")
		assert.Contains(ct, strings.ToLower(cmd), "otel-agent.exe")
		assert.Contains(ct, strings.ToLower(cmd), "ddot")
		pid = p
	}, timeout, 2*time.Second)
	return pid
}

// TestDDOTReloadAfterYamlChange edits processes.d while DDOT runs under dd-procmgrd,
// runs reload, and asserts the collector respawns (new PID). Parity with Linux TestDDOTReloadAfterYamlChange.
func (s *procmgrWindowsSuite) TestDDOTReloadAfterYamlChange() {
	s.requireDDOTWindows()

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	yamlPath := filepath.Join(installPath, "processes.d", "datadog-agent-ddot.yaml")

	originalPID := s.waitWindowsDDOTRunning(90 * time.Second)

	s.T().Cleanup(func() {
		_, _ = s.Env().RemoteHost.Execute(psRemote(
			`$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $c=$c.Replace('%s','%s'); $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)`,
			yamlPath, windowsDDOTDescE2ELine, windowsDDOTDescOriginalLine,
		))
		_, _ = s.Env().RemoteHost.Execute(s.platform.cliCmd("reload"))
	})

	s.Env().RemoteHost.MustExecute(psRemote(
		`$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $c=$c.Replace('%s','%s'); $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)`,
		yamlPath, windowsDDOTDescOriginalLine, windowsDDOTDescE2ELine,
	))

	reloadOut := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("reload"))
	assert.Contains(s.T(), reloadOut, "datadog-agent-ddot", "reload output: %s", reloadOut)
	assert.Contains(s.T(), reloadOut, "Modified", "reload output: %s", reloadOut)

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.platform.cliCmd("describe datadog-agent-ddot"))
		assertField(ct, out, "State", "Running")
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(ct, p) || !assert.NotEqual(ct, "-", p) {
			return
		}
		assert.NotEqual(ct, originalPID, p, "DDOT should respawn with a new PID after reload")
	}, 90*time.Second, 2*time.Second)

	out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe datadog-agent-ddot"))
	assertField(s.T(), out, "Description", "E2E-reload-after-yaml")
}

// ---------------------------------------------------------------------------
// Windows-only: ADP tests
// ---------------------------------------------------------------------------

// trySetupWindowsADPForProcmgr enables ADP in datadog.yaml and ensures processes.d
// datadog-agent-data-plane.yaml exists (MSI install writes it when agent-data-plane.exe
// is present). Skips (hasADP=false) when the ADP binary is unavailable on this image.
func (s *procmgrWindowsSuite) trySetupWindowsADPForProcmgr() bool {
	s.T().Helper()
	host := s.Env().RemoteHost

	installPath, err := windowsagent.GetInstallPathFromRegistry(host)
	if err != nil {
		s.T().Logf("windows adp procmgr: InstallPath registry: %v", err)
		return false
	}
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	if err != nil {
		s.T().Logf("windows adp procmgr: ConfigRoot registry: %v", err)
		return false
	}

	adpExe := joinWindowsPath(installPath, "bin", "agent", "agent-data-plane.exe")
	if _, err := host.Execute(s.platform.checkBinCmd(adpExe)); err != nil {
		s.T().Logf("windows adp procmgr: no agent-data-plane at %s", adpExe)
		return false
	}

	fleetPolicies := joinWindowsPath(configRoot, "Installer", "managed", "datadog-agent", "stable")
	runDir := joinWindowsPath(configRoot, "run")
	yamlPath := joinWindowsPath(installPath, "processes.d", "datadog-agent-data-plane.yaml")
	host.MustExecute(ensureWindowsDirPS(joinWindowsPath(installPath, "processes.d")))
	host.MustExecute(ensureWindowsDirPS(fleetPolicies))
	host.MustExecute(ensureWindowsDirPS(runDir))

	yamlBody := windowsADPProcmgrYAMLContent(installPath, configRoot, fleetPolicies)
	b64 := base64.StdEncoding.EncodeToString([]byte(yamlBody))
	if _, err := host.Execute(psRemote(
		`$ErrorActionPreference='Stop'; $b=[Convert]::FromBase64String('%s'); [IO.File]::WriteAllBytes('%s', $b)`,
		b64, yamlPath,
	)); err != nil {
		s.T().Logf("windows adp procmgr: write processes.d yaml failed: %v", err)
		return false
	}

	datadogYAML := joinWindowsPath(configRoot, "datadog.yaml")
	appendADP := "\nprocess_manager:\n  enabled: true\ndata_plane:\n  enabled: true\n"
	b64AppendADP := base64.StdEncoding.EncodeToString([]byte(appendADP))
	host.MustExecute(psRemote(
		`$dy='%s'; if (-not (Test-Path -LiteralPath $dy)) { exit 0 }; if (-not (Select-String -LiteralPath $dy -Pattern 'data_plane:' -Quiet)) { $a=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); Add-Content -LiteralPath $dy -Value $a -Encoding utf8 }`,
		datadogYAML, b64AppendADP,
	))
	// ADP reads data_plane.enabled from the core Agent config stream, not only datadog.yaml.
	host.MustExecute(psRemote(
		`[Environment]::SetEnvironmentVariable('DD_PROCESS_MANAGER_ENABLED','true','Machine'); [Environment]::SetEnvironmentVariable('DD_DATA_PLANE_ENABLED','true','Machine')`,
	))
	if err := windowsCommon.RestartService(host, "DatadogAgent"); err != nil {
		s.T().Logf("windows adp procmgr: restart DatadogAgent failed: %v", err)
		return false
	}
	procmgrReady := assert.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := host.MustExecuteOn(ct, s.platform.checkSvcRunning)
		assert.Equal(ct, s.platform.svcRunningOutput, strings.TrimSpace(out))
	}, 90*time.Second, 2*time.Second)
	if !procmgrReady {
		s.T().Logf("windows adp procmgr: dd-procmgr-service not running after DatadogAgent restart")
		return false
	}

	if _, err := host.Execute(s.platform.cliCmd("reload")); err != nil {
		s.T().Logf("windows adp procmgr: initial reload failed: %v", err)
		return false
	}

	return true
}

func windowsADPProcmgrYAMLContent(installPath, configRoot, fleetPolicies string) string {
	config := embedded.ADPWindowsProcmgrConfig
	config = strings.ReplaceAll(config, "__ADP_INSTALL_ROOT__", toWindowsSlashPath(installPath))
	config = strings.ReplaceAll(config, "__ADP_ETC_ROOT__", toWindowsSlashPath(configRoot))
	config = strings.ReplaceAll(config, "__ADP_FLEET_POLICIES_DIR__", toWindowsSlashPath(fleetPolicies))
	return config
}

func (s *procmgrWindowsSuite) logWindowsADPDiagnostics() {
	s.T().Helper()
	host := s.Env().RemoteHost

	describe, _ := host.Execute(s.platform.cliCmd("describe " + adpProcessName))
	s.T().Logf("dd-procmgr describe %s:\n%s", adpProcessName, describe)

	installPath, err := windowsagent.GetInstallPathFromRegistry(host)
	if err != nil {
		s.T().Logf("windows adp diagnostics: InstallPath registry: %v", err)
		return
	}
	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	if err != nil {
		s.T().Logf("windows adp diagnostics: ConfigRoot registry: %v", err)
		return
	}

	agentExe := joinWindowsPath(installPath, "bin", "agent.exe")
	for _, key := range []string{"data_plane.enabled", "process_manager.enabled"} {
		out, err := host.Execute(fmt.Sprintf(`& "%s" config get %s -s`, agentExe, key))
		if err != nil {
			s.T().Logf("agent config get %s: %v", key, err)
			continue
		}
		s.T().Logf("agent config get %s: %s", key, strings.TrimSpace(out))
	}

	adpLog := joinWindowsPath(configRoot, "logs", "agent-data-plane.log")
	logTail, err := host.Execute(psRemote(
		`$p='%s'; if (Test-Path -LiteralPath $p) { Get-Content -LiteralPath $p -Tail 80 } else { 'log file not found: ' + $p }`,
		adpLog,
	))
	if err != nil {
		s.T().Logf("agent-data-plane.log tail (%s): %v", adpLog, err)
		return
	}
	s.T().Logf("agent-data-plane.log tail (%s):\n%s", adpLog, logTail)
}

func (s *procmgrWindowsSuite) requireADPWindows() {
	s.T().Helper()
	if !s.hasADP {
		s.T().Skip("windows adp procmgr: agent-data-plane.exe or processes.d bootstrap not available on this image")
	}
	s.requireCLI()
}

func (s *procmgrWindowsSuite) waitWindowsADPRunning(timeout time.Duration) string {
	s.T().Helper()
	var pid string
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.platform.cliCmd("describe "+adpProcessName))
		assertField(ct, out, "State", "Running")
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(ct, p) || !assert.NotEqual(ct, "-", p) {
			return
		}
		cmd := fieldValue(out, "Command")
		assert.Contains(ct, strings.ToLower(cmd), "agent-data-plane.exe")
		pid = p
	}, timeout, 2*time.Second)
	return pid
}

func (s *procmgrWindowsSuite) getWindowsRestartCount(name string) int {
	s.T().Helper()
	out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe " + name))
	count, err := strconv.Atoi(fieldValue(out, "Restarts"))
	require.NoError(s.T(), err, "Restarts field for %s should be a number", name)
	return count
}

func (s *procmgrWindowsSuite) TestADPProcessRunning() {
	s.requireADPWindows()

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	configRoot, err := windowsagent.GetConfigRootFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)

	pid := s.waitWindowsADPRunning(90 * time.Second)

	pidFile := joinWindowsPath(configRoot, "run", "agent-data-plane.pid")
	pidFileContent := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		psRemote(`(Get-Content -Raw -LiteralPath '%s').Trim()`, pidFile),
	))
	assert.Equal(s.T(), pid, pidFileContent, "PID file should match procmgrd-reported PID")

	adpExe := joinWindowsPath(installPath, "bin", "agent", "agent-data-plane.exe")
	_, err = s.Env().RemoteHost.Execute(s.platform.checkBinCmd(adpExe))
	assert.NoError(s.T(), err, "ADP binary should exist at %s", adpExe)
}

func (s *procmgrWindowsSuite) TestADPRestartAfterKill() {
	s.requireADPWindows()

	originalPID := s.waitWindowsADPRunning(90 * time.Second)
	baselineRestarts := s.getWindowsRestartCount(adpProcessName)

	pidNum, err := strconv.ParseUint(originalPID, 10, 32)
	require.NoError(s.T(), err)
	s.Env().RemoteHost.MustExecute(s.platform.killPIDCmd(uint32(pidNum)))

	newPID := s.waitWindowsADPRunning(60 * time.Second)
	require.NotEqual(s.T(), originalPID, newPID,
		"PID should differ after restart (was %s)", originalPID)
	assert.Equal(s.T(), baselineRestarts+1, s.getWindowsRestartCount(adpProcessName),
		"Restarts should have increased by 1 (baseline %d)", baselineRestarts)
}

func (s *procmgrWindowsSuite) TestADPProcessDescribe() {
	s.requireADPWindows()

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	expectedCmd := joinWindowsPath(installPath, "bin", "agent", "agent-data-plane.exe")

	ok := assert.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.platform.cliCmd("describe "+adpProcessName))
		assertField(ct, out, "Name", adpProcessName)
		assertField(ct, out, "State", "Running")
		assert.Equal(ct, expectedCmd, toWindowsSlashPath(fieldValue(out, "Command")))
		assertField(ct, out, "Restart Policy", "on-failure")
		assertHasField(ct, out, "PID")
		assertHasField(ct, out, "UUID")
	}, 90*time.Second, 2*time.Second)
	if !ok {
		s.logWindowsADPDiagnostics()
	}
}

func (s *procmgrWindowsSuite) TestADPReloadAfterYamlChange() {
	s.requireADPWindows()

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	yamlPath := joinWindowsPath(installPath, "processes.d", "datadog-agent-data-plane.yaml")

	originalPID := s.waitWindowsADPRunning(90 * time.Second)

	s.T().Cleanup(func() {
		_, _ = s.Env().RemoteHost.Execute(psRemote(
			`$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $c=$c.Replace('%s','%s'); $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)`,
			yamlPath, windowsADPDescE2ELine, windowsADPDescOriginalLine,
		))
		_, _ = s.Env().RemoteHost.Execute(s.platform.cliCmd("reload"))
	})

	s.Env().RemoteHost.MustExecute(psRemote(
		`$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $c=$c.Replace('%s','%s'); $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)`,
		yamlPath, windowsADPDescOriginalLine, windowsADPDescE2ELine,
	))

	reloadOut := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("reload"))
	assert.Contains(s.T(), reloadOut, adpProcessName, "reload output: %s", reloadOut)
	assert.Contains(s.T(), reloadOut, "Modified", "reload output: %s", reloadOut)

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.platform.cliCmd("describe "+adpProcessName))
		assertField(ct, out, "State", "Running")
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(ct, p) || !assert.NotEqual(ct, "-", p) {
			return
		}
		assert.NotEqual(ct, originalPID, p, "ADP should respawn with a new PID after reload")
	}, 90*time.Second, 2*time.Second)

	out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe " + adpProcessName))
	assertField(s.T(), out, "Description", "E2E-reload-after-yaml")
}
