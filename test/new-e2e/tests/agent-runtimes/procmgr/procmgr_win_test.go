// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	e2ecomponents "github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
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

	// Static suite fixtures live under processes.d as YAML (see TestProcmgrSmokeWindowsSuite).
	// Runtime create is for per-test definitions (unique names, RPC auth checks, etc.).

	winMissingBinaryConfig = `command: C:\nonexistent\binary.exe
condition_path_exists: C:\nonexistent\binary.exe
auto_start: true
restart: never
description: should not start
`

	winUserProfileEnvMarkerDir = `C:/ProgramData/Datadog`

	adpProcessName = "datadog-agent-data-plane"
)

// psRemote builds a PowerShell script for RemoteHost.Execute; string args are escaped for single-quoted literals.
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

// Path helpers for remote PowerShell: the e2e runner is Linux/macOS, so registry paths
// need explicit slash normalization instead of filepath.Join.
func toWindowsSlashPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, `/`)
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

func joinWindowsPath(base string, elems ...string) string {
	parts := make([]string, 0, len(elems)+1)
	parts = append(parts, strings.TrimRight(toWindowsSlashPath(base), `/`))
	parts = append(parts, elems...)
	return strings.Join(parts, "/")
}

func ensureWindowsDirPS(dir string) string {
	return psRemote(`New-Item -ItemType Directory -Force -Path '%s' | Out-Null`, dir)
}

// withADPEnabled enables ADP via datadog.yaml during provisioning; the provisioner restarts
// DatadogAgent afterward, which also starts dd-procmgr-service when process_manager is enabled.
func withADPEnabled() agentparams.Option {
	return func(p *agentparams.Params) error {
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.String("data_plane.enabled: true"))
		return nil
	}
}

var winPlatform = platformConfig{
	daemonBin:         winDaemonBin,
	cliBin:            winCLIBin,
	configDir:         winConfigDir,
	sleepCommand:      winSleepCommand,
	testProcessYAML:   winTestProcessConfig,
	missingBinaryYAML: winMissingBinaryConfig,
	checkFileExists: func(path string) string {
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
}

func TestProcmgrSmokeWindowsSuite(t *testing.T) {
	t.Parallel()
	s := &procmgrWindowsSuite{}
	s.platform = winPlatform
	e2e.Run(t, s, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault), ec2.WithInternetAccess()),
				ec2.WithAgentOptions(
					agentparams.WithFile(winConfigDir+"/test-sleep.yaml", winTestProcessConfig, true),
					agentparams.WithFile(winConfigDir+"/missing-binary.yaml", winMissingBinaryConfig, true),
					withADPEnabled(),
				),
			),
		),
	))
}

func (s *procmgrWindowsSuite) SetupSuite() {
	s.baseProcmgrSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// dd-procmgr-service is DEMAND_START; start it explicitly before tests.
	s.ensureWindowsProcmgrServiceRunning()
}

// TearDownTest restores dd-procmgr-service after tests that restart DatadogAgent.
// Stopping DatadogAgent stops dependent services including dd-procmgr-service.
func (s *procmgrWindowsSuite) TearDownTest() {
	s.ensureWindowsProcmgrServiceRunning()
}

func (s *procmgrWindowsSuite) ensureWindowsProcmgrServiceRunning() {
	host := s.Env().RemoteHost
	host.MustExecute(`powershell -Command "Start-Service dd-procmgr-service"`)
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := host.MustExecuteOn(t, s.platform.checkSvcRunning)
		assert.Equal(t, s.platform.svcRunningOutput, strings.TrimSpace(out))
	}, 60*time.Second, 2*time.Second)
}

func (s *procmgrWindowsSuite) TestAdministratorCreateViaNamedPipe() {
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

func (s *procmgrWindowsSuite) TestDDOTReloadAfterYamlChange() {
	s.requireDDOTWindows()

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	assert.NotContains(s.T(), strings.ToUpper(caller), "SYSTEM")

	procName := fmt.Sprintf("e2e-admin-pipe-create-%d", time.Now().UnixNano())
	createOut, err := host.Execute(s.cliCreate(procmgrCreateSpec{
		Name:        procName,
		Command:     `C:\Windows\System32\cmd.exe`,
		Args:        []string{"/c", "exit"},
		Description: "E2E admin pipe auth",
		NoAutoStart: true,
	}))
	require.NoError(s.T(), err, "Create RPC should succeed for Administrator pipe client; output: %s", createOut)
	assert.NotContains(s.T(), strings.ToLower(createOut), "permission denied")
	assert.Contains(s.T(), createOut, "UUID:")

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

func (s *procmgrWindowsSuite) TestAgentProfileChildRunsAsAgentUser() {
	host := s.Env().RemoteHost

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		desc := host.MustExecuteOn(ct, s.cliDescribe("test-sleep"))
		assertField(ct, desc, "State", "Running")
		pid := fieldValue(desc, "PID")
		if !assert.NotEmpty(ct, pid) {
			return
		}
		owner, err := windowsProcessOwnerByPID(host, pid)
		assert.NoError(ct, err)
		assert.NotContains(ct, owner, "NT AUTHORITY/SYSTEM")
	}, 60*time.Second, 2*time.Second)
}

func (s *procmgrWindowsSuite) TestAgentProfileDescribeUserMatchesRuntimeUser() {
	host := s.Env().RemoteHost

	_, agentUser, err := windowsagent.GetAgentUserFromRegistry(host)
	require.NoError(s.T(), err)

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		desc := host.MustExecuteOn(ct, s.cliDescribe("test-sleep"))
		assertField(ct, desc, "State", "Running")
		assertField(ct, desc, "Profile", "agent")
		assertHasField(ct, desc, "User")
		assertHasField(ct, desc, "Runtime User")

		user := fieldValue(desc, "User")
		runtimeUser := fieldValue(desc, "Runtime User")
		assert.NotEmpty(ct, user)
		assert.NotEmpty(ct, runtimeUser)
		assert.Equal(ct, user, runtimeUser,
			"describe User should match Runtime User for agent-profile children")
		// MSI stores the machine name as installedDomain for local ddagentuser;
		// procmgr display normalizes local SAM accounts to .\user.
		assert.Equal(ct, `.\`+agentUser, user,
			"local agent user should use registry-style .\\user display")
	}, 60*time.Second, 2*time.Second)
}

func (s *procmgrWindowsSuite) TestAgentProfileChildHasUserProfileEnv() {
	host := s.Env().RemoteHost

	_, agentUser, err := windowsagent.GetAgentUserFromRegistry(host)
	require.NoError(s.T(), err)

	runID := time.Now().UnixNano()
	procName := fmt.Sprintf("e2e-userprofile-env-%d", runID)
	markerPath := windowsUserProfileEnvMarkerPath(runID)

	createOut, err := host.Execute(s.cliCreate(windowsUserProfileEnvCreateSpec(procName, markerPath)))
	require.NoError(s.T(), err, "Create should spawn an agent-profile child; output: %s", createOut)
	assert.Contains(s.T(), createOut, "UUID:")

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		desc := host.MustExecuteOn(ct, s.cliDescribe(procName))
		assertField(ct, desc, "Profile", "agent")

		out := host.MustExecuteOn(ct, psRemote(`Get-Content -LiteralPath '%s' -ErrorAction Stop`, markerPath))
		userProfile := strings.TrimSpace(out)
		assert.NotEmpty(ct, userProfile)
		assert.NotContains(ct, strings.ToLower(userProfile), "systemprofile")
		assert.Contains(ct, strings.ToLower(userProfile), strings.ToLower(agentUser))
	}, 120*time.Second, 2*time.Second)
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
	out := s.Env().RemoteHost.MustExecute(s.cliDescribe(name))
	count, err := strconv.Atoi(fieldValue(out, "Restarts"))
	require.NoError(s.T(), err, "Restarts field for %s should be a number", name)
	return count
}

func (s *procmgrWindowsSuite) TestADPProcessRunning() {
	pid := s.waitWindowsADPRunning(90 * time.Second)

	configRoot, err := windowsagent.GetConfigRootFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), pid, strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		psRemote(`(Get-Content -Raw -LiteralPath '%s').Trim()`, joinWindowsPath(configRoot, "run", "agent-data-plane.pid")),
	)), "PID file should match procmgrd-reported PID")

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	s.Env().RemoteHost.MustExecute(s.platform.checkFileExists(
		joinWindowsPath(installPath, "bin", "agent", "agent-data-plane.exe"),
	))
}

func (s *procmgrWindowsSuite) TestADPCOATTelemetry() {
	s.waitWindowsADPRunning(90 * time.Second)

	// The COAT reporter refreshes periodically, so wait for its snapshot to include
	// the ADP process state already confirmed through dd-procmgr.
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		output := s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "agent-full-telemetry"}))
		assert.True(ct, telemetryGaugeIsTrue(output, "runtime__procmgr_process_running", map[string]string{
			"process": adpProcessName,
		}), "ADP procmgr running gauge should be emitted: %s", output)
		assert.True(ct, telemetryGaugeIsTrue(output, "runtime__agent_service_installed", map[string]string{
			"service": "agent-data-plane",
		}), "ADP installed gauge should be emitted: %s", output)
		assert.True(ct, telemetryGaugeIsTrue(output, "runtime__agent_service_procmgr_configured", map[string]string{
			"service": "agent-data-plane",
		}), "ADP configured gauge should be emitted: %s", output)
		assert.True(ct, telemetryGaugeIsTrue(output, "runtime__agent_service_management_mode", map[string]string{
			"service": "agent-data-plane",
			"mode":    "procmgr",
		}), "ADP procmgr management-mode gauge should be emitted: %s", output)
	}, 7*time.Minute, 10*time.Second)
}

func telemetryGaugeIsTrue(output, metric string, labels map[string]string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, metric) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[len(fields)-1] != "1" && fields[len(fields)-1] != "1.0") {
			continue
		}

		allLabelsMatch := true
		for key, value := range labels {
			if !strings.Contains(line, key+`="`+value+`"`) {
				allLabelsMatch = false
				break
			}
		}
		if allLabelsMatch {
			return true
		}
	}
	return false
}

func (s *procmgrWindowsSuite) TestADPRestartAfterKill() {
	originalPID := s.waitWindowsADPRunning(90 * time.Second)
	baselineRestarts := s.getWindowsRestartCount(adpProcessName)

	pid, err := strconv.ParseUint(originalPID, 10, 32)
	require.NoError(s.T(), err)
	s.Env().RemoteHost.MustExecute(s.platform.killPIDCmd(uint32(pid)))

	newPID := s.waitWindowsADPRunning(60 * time.Second)
	require.NotEqual(s.T(), originalPID, newPID, "PID should differ after restart (was %s)", originalPID)
	assert.Equal(s.T(), baselineRestarts+1, s.getWindowsRestartCount(adpProcessName),
		"Restarts should have increased by 1 (baseline %d)", baselineRestarts)
}

func (s *procmgrWindowsSuite) TestADPProcessDescribe() {
	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliDescribe(adpProcessName))
		assertField(ct, out, "Name", adpProcessName)
		assertField(ct, out, "State", "Running")
		assert.Equal(ct,
			joinWindowsPath(installPath, "bin", "agent", "agent-data-plane.exe"),
			toWindowsSlashPath(fieldValue(out, "Command")),
		)
		assertField(ct, out, "Restart Policy", "on-failure")
		assertHasField(ct, out, "PID")
		assertHasField(ct, out, "UUID")
	}, 90*time.Second, 2*time.Second)
}

func (s *procmgrWindowsSuite) TestADPReloadAfterYamlChange() {
	s.requireCLI()
	originalPID := s.waitWindowsADPRunning(90 * time.Second)

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	yamlPath := joinWindowsPath(installPath, "processes.d", "datadog-agent-data-plane.yaml")

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

	assertField(s.T(),
		s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe "+adpProcessName)),
		"Description", "E2E-reload-after-yaml",
	)
	out, err := host.Execute(script)
	return strings.TrimSpace(out), err
}
