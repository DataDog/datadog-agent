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

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
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
# Start-Sleep receives CTRL_BREAK and keeps sleeping, so every stop of this
# fixture goes through the force-kill path. Bounded here so a stop costs ten
# seconds rather than the ninety-second default.
stop_timeout: 10
description: E2E test process
`

	winMissingBinaryConfig = `command: C:\nonexistent\binary.exe
condition_path_exists: C:\nonexistent\binary.exe
auto_start: true
restart: never
description: should not start
`

	// Description line from fleet/embedded DDOT template; reload tests mutate it.
	windowsDDOTDescOriginalLine = "description: Datadog Distribution of OpenTelemetry Collector"
	windowsDDOTDescE2ELine      = "description: E2E-reload-after-yaml"

	windowsADPDescOriginalLine = "description: Datadog Agent Data Plane"
	windowsADPDescE2ELine      = "description: E2E-reload-after-yaml"

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

// agentBin returns installRoot/bin/agent/<name> with Windows-friendly separators.
func agentBin(installRoot, name string) string {
	return joinWindowsPath(installRoot, "bin", "agent", name)
}

// processesDConfig returns installRoot/processes.d/<name>.
func processesDConfig(installRoot, name string) string {
	return joinWindowsPath(installRoot, "processes.d", name)
}

// requireHostPath fails the test when path is missing on the remote host.
func requireHostPath(t *testing.T, host *components.RemoteHost, path, msg string) {
	t.Helper()
	exists, err := host.FileExists(path)
	require.NoError(t, err)
	require.True(t, exists, msg, path)
}

// skipUnlessHostPath skips the test when path is missing on the remote host.
func skipUnlessHostPath(t *testing.T, host *components.RemoteHost, path, reason string) {
	t.Helper()
	exists, err := host.FileExists(path)
	require.NoError(t, err)
	if !exists {
		t.Skip(reason)
	}
}

// waitProcmgrStableFor is how long the same PID must stay Running before waitProcmgrRunning
// accepts it, so a restart blip is not treated as steady state.
const waitProcmgrStableFor = 5 * time.Second

// procmgrHoldFor is how long the cutover assertions must keep seeing the state they want,
// rather than accepting the first poll that shows it. waitProcmgrStableFor only rejects a
// restart blip during setup. This is several times the five second sleep system-probe takes
// before exiting when no module is enabled, so that exit cannot satisfy a check, and it
// matches the window AssertProcmgrProcessRunning uses.
const procmgrHoldFor = 20 * time.Second

// waitProcmgrRunning polls dd-procmgr describe until name is Running with the same real PID
// for waitProcmgrStableFor, so a brief Running blip during a restart is not accepted.
func waitProcmgrRunning(t *testing.T, host *components.RemoteHost, cli, name string, timeout time.Duration) string {
	t.Helper()
	var (
		pid         string
		stableSince time.Time
	)
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		out, err := host.Execute(procmgrCmd(cli, "describe "+name))
		if !assert.NoError(ct, err) {
			pid = ""
			return
		}
		if !assert.Equal(ct, "Running", fieldValue(out, "State"), "process %s: %s", name, out) {
			pid = ""
			return
		}
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(ct, p, "PID should be present for a Running process") ||
			!assert.NotEqual(ct, "-", p, "PID should not be '-' for a Running process") {
			pid = ""
			return
		}
		now := time.Now()
		if p != pid {
			pid = p
			stableSince = now
		}
		assert.GreaterOrEqual(ct, now.Sub(stableSince), waitProcmgrStableFor,
			"process %s PID %s not stable for %s yet (seen for %s)", name, p, waitProcmgrStableFor, now.Sub(stableSince).Round(time.Second))
	}, timeout, 3*time.Second)
	return pid
}

// respawnProcmgrRunning stops and starts a process, then waits for the replacement to be
// Running.
//
// The CLI call itself is not required to succeed. A dd-procmgr RPC can fail at the
// transport level while the daemon carries the spawn out anyway, and retrying `start`
// then reports the process as already running. What a caller needs is the end state, so
// that is what is asserted; the command's own error is only logged, where it shows up in
// the failure output if the process does not come back.
//
// Running alone is not that end state: a respawn that fails before the stop lands leaves
// the old process running, which the child of a config or environment change the caller
// just made would not reflect. The PID has to move for the spawn path to have run.
func respawnProcmgrRunning(t *testing.T, host *components.RemoteHost, cli, name string, timeout time.Duration) {
	t.Helper()
	replaced, err := procmgrPIDToReplace(host, cli, name)
	require.NoError(t, err)
	if _, err := host.Execute(procmgrRespawn(cli, name)); err != nil {
		t.Logf("respawn of %s reported: %v", name, err)
	}
	pid := waitProcmgrRunning(t, host, cli, name, timeout)
	// A process that was not running has no PID to replace, which any real PID satisfies.
	require.NotEqual(t, replaced, pid,
		"%s is still PID %s, the process the respawn was meant to replace", name, replaced)
}

// restoreProcmgrRunning is respawnProcmgrRunning for a cleanup: it reports a process that
// never comes back without ever calling FailNow, which a cleanup must not do.
func restoreProcmgrRunning(t *testing.T, host *components.RemoteHost, cli, name string, timeout time.Duration) {
	t.Helper()
	replaced, err := procmgrPIDToReplace(host, cli, name)
	if err != nil {
		t.Errorf("failed to read the PID %s is restarting from: %v", name, err)
	}
	if _, err := host.Execute(procmgrRespawn(cli, name)); err != nil {
		t.Logf("respawn of %s reported: %v", name, err)
	}
	assertProcmgrReplaced(t, host, cli, name, replaced, timeout)
}

// procmgrDescribeRetryFor is how long procmgrPIDToReplace keeps retrying a describe that
// cannot be read, which is a transport failure rather than an answer.
const procmgrDescribeRetryFor = 30 * time.Second

// procmgrPIDToReplace reads the PID a respawn is about to replace, or "" when the process
// is not running and so has none.
//
// An unreadable describe is retried rather than reported as "", since "" turns the caller's
// replacement check back into a Running check, and a Running check is satisfied by the very
// process the respawn was meant to replace. Only a describe that never becomes readable is
// an error, and that is a broken host rather than a transient RPC failure.
func procmgrPIDToReplace(host *components.RemoteHost, cli, name string) (string, error) {
	deadline := time.Now().Add(procmgrDescribeRetryFor)
	for {
		var reason error
		out, err := host.Execute(procmgrCmd(cli, "describe "+name))
		switch {
		case err != nil:
			reason = err
		case fieldValue(out, "State") != "Running":
			return "", nil
		default:
			pid := fieldValue(out, "PID")
			if pid != "" && pid != "-" {
				return pid, nil
			}
			reason = fmt.Errorf("no PID for a Running process: %s", out)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("describe %s stayed unreadable for %s: %w", name, procmgrDescribeRetryFor, reason)
		}
		time.Sleep(3 * time.Second)
	}
}

// assertProcmgrRunning waits for a process to be Running without ever calling FailNow,
// which a cleanup must not do. A cleanup that only issues the commands and reports
// success leaves the next test, which may run on this same host, to discover that the
// process never came back.
func assertProcmgrRunning(t *testing.T, host *components.RemoteHost, cli, name string, timeout time.Duration) {
	t.Helper()
	assertProcmgrReplaced(t, host, cli, name, "", timeout)
}

// assertProcmgrReplaced is assertProcmgrRunning for a respawn: replacedPID is the process
// the respawn was meant to replace, so seeing it again means the spawn path never ran and
// the child still carries whatever the test installed. Pass "" when nothing was running
// beforehand, which leaves no PID to rule out and asserts Running alone.
func assertProcmgrReplaced(t *testing.T, host *components.RemoteHost, cli, name, replacedPID string, timeout time.Duration) {
	t.Helper()
	assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		out, err := host.Execute(procmgrCmd(cli, "describe "+name))
		if !assert.NoError(ct, err) {
			return
		}
		if !assert.Equal(ct, "Running", fieldValue(out, "State"),
			"%s should be Running again after cleanup: %s", name, out) {
			return
		}
		if replacedPID != "" {
			assert.NotEqual(ct, replacedPID, fieldValue(out, "PID"),
				"%s is still PID %s, the process the cleanup was meant to replace: %s", name, replacedPID, out)
		}
	}, timeout, 3*time.Second)
}

// requireSupervisedOnlyByProcmgr requires that dd-procmgr is the one thing running name, for
// procmgrHoldFor: name is Running under dd-procmgr as wantPID, and legacyService, which used to
// run it, is Stopped or not registered. timeout is the deadline for reaching that hold, not the
// length of it.
//
// Both are read on every tick and share one window, because "only" is a claim about the pair.
// Given a window each in turn, the workload could restart during the service half and the test
// would still pass, which is exactly what this rules out: a workload run twice, or not at all.
//
// A restart, or a legacy service in any state but Stopped, resets the hold like any other
// mismatch, so the run keeps polling to the deadline and reports what it last saw. Neither can
// be undone by waiting, but aborting the moment one appears would mean either a sleep-driven
// loop of our own or FailNow from the condition, which runs on another goroutine where it only
// stops the worker and leaves the tick reporting success.
func requireSupervisedOnlyByProcmgr(t *testing.T, host *components.RemoteHost, cli, name, wantPID, legacyService string, timeout time.Duration) {
	t.Helper()
	require.NotEmpty(t, wantPID, "wantPID must be set (capture it with waitProcmgrRunning)")

	serviceQuery := fmt.Sprintf(
		`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`,
		legacyService,
	)

	var heldSince time.Time
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		describe, err := host.Execute(procmgrCmd(cli, "describe "+name))
		if !assert.NoError(ct, err, "dd-procmgr describe %s", name) {
			heldSince = time.Time{}
			return
		}
		status, err := host.Execute(serviceQuery)
		if !assert.NoError(ct, err, "Get-Service %s", legacyService) {
			heldSince = time.Time{}
			return
		}

		state, pid := fieldValue(describe, "State"), fieldValue(describe, "PID")
		serviceState := strings.TrimSpace(status)
		runningAsWantPID := assert.Equal(ct, "Running", state, "process %s: %s", name, describe) &&
			assert.Equal(ct, wantPID, pid,
				"process %s should still be the auto-spawned PID: %s", name, describe)
		legacyDown := assert.Contains(ct, []string{"Stopped", "Absent"}, serviceState,
			"%s must stay down while dd-procmgr supervises %s", legacyService, name)
		if !runningAsWantPID || !legacyDown {
			heldSince = time.Time{}
			return
		}

		now := time.Now()
		if heldSince.IsZero() {
			heldSince = now
		}
		// A tick that records no failure is an immediate success, so the hold has to be an
		// assertion rather than a silent early return.
		assert.GreaterOrEqual(ct, now.Sub(heldSince), procmgrHoldFor,
			"%s has held PID %s with %s down for %s, needs %s",
			name, wantPID, legacyService, now.Sub(heldSince).Round(time.Second), procmgrHoldFor)
	}, timeout, 3*time.Second)
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
	require.NoError(s.T(), windowsCommon.StartService(s.Env().RemoteHost, "dd-procmgr-service"))

	if s.hasCLI {
		require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
			out := s.Env().RemoteHost.MustExecuteOn(t,
				`powershell -Command "(Get-Service dd-procmgr-service).Status"`)
			assert.Equal(t, "Running", strings.TrimSpace(out))
		}, 60*time.Second, 2*time.Second)
	}

	s.tryInstallWindowsDDOTForProcmgr()
}

// tryInstallWindowsDDOTForProcmgr bootstraps DDOT under procmgr when embedded otel-agent is on the image.
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

	yamlPath := processesDConfig(installPath, "datadog-agent-ddot.yaml")
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
	yamlPath := processesDConfig(installPath, "datadog-agent-ddot.yaml")

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
	s.requireCLI()
	pid := s.waitWindowsADPRunning(90 * time.Second)

	configRoot, err := windowsagent.GetConfigRootFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), pid, strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		psRemote(`(Get-Content -Raw -LiteralPath '%s').Trim()`, joinWindowsPath(configRoot, "run", "agent-data-plane.pid")),
	)), "PID file should match procmgrd-reported PID")

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	s.Env().RemoteHost.MustExecute(s.platform.checkBinCmd(
		agentBin(installPath, "agent-data-plane.exe"),
	))
}

func (s *procmgrWindowsSuite) TestADPCOATTelemetry() {
	s.requireCLI()
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
		assertAgentServiceRunningExclusive(ct, output, "agent-data-plane", "procmgr")
	}, 7*time.Minute, 10*time.Second)
}

func (s *procmgrWindowsSuite) TestADPRestartAfterKill() {
	s.requireCLI()
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
	s.requireCLI()
	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.platform.cliCmd("describe "+adpProcessName))
		assertField(ct, out, "Name", adpProcessName)
		assertField(ct, out, "State", "Running")
		assert.Equal(ct,
			agentBin(installPath, "agent-data-plane.exe"),
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
	yamlPath := processesDConfig(installPath, "datadog-agent-data-plane.yaml")

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
}
