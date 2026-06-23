// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
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
)

var winPlatform = platformConfig{
	daemonBin:         winDaemonBin,
	cliBin:            winCLIBin,
	configDir:         winConfigDir,
	sleepCommand:      winSleepCommand,
	testProcessYAML:   winTestProcessConfig,
	missingBinaryYAML: winMissingBinaryConfig,
	checkBinCmd: func(path string) string {
		return fmt.Sprintf(`powershell -Command "if (Test-Path '%s') { exit 0 } else { exit 1 }"`, path)
	},
	checkSvcRunning:  `powershell -Command "(Get-Service dd-procmgr-service).Status"`,
	svcRunningOutput: "Running",
	cliCmd: func(args string) string {
		return fmt.Sprintf(`& "%s" %s`, winCLIBin, args)
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
	copyPS := fmt.Sprintf(
		`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; New-Item -ItemType Directory -Force -LiteralPath '%s' | Out-Null; Copy-Item -LiteralPath '%s' -Destination '%s' -Force"`,
		psEscapeSingleQuotedPath(destDir),
		psEscapeSingleQuotedPath(embeddedOtel),
		psEscapeSingleQuotedPath(destExe),
	)
	if _, err := host.Execute(copyPS); err != nil {
		s.T().Logf("windows ddot procmgr: copy otel-agent failed: %v", err)
		return
	}

	exExample := filepath.Join(configRoot, "otel-config.yaml.example")
	exOut := filepath.Join(configRoot, "otel-config.yaml")
	otelPS := fmt.Sprintf(
		`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; $ex='%s'; $out='%s'; if (Test-Path -LiteralPath $ex) { $c = Get-Content -Raw -LiteralPath $ex; $c = $c -replace '\$\{env:DD_API_KEY\}','aaaaaaaaaaaaaaaa'; $c = $c -replace '\$\{env:DD_SITE\}','datadoghq.com'; Set-Content -LiteralPath $out -Value $c -Encoding utf8 } elseif (-not (Test-Path -LiteralPath $out)) { throw 'missing otel-config' }"`,
		psEscapeSingleQuotedPath(exExample),
		psEscapeSingleQuotedPath(exOut),
	)
	if _, err := host.Execute(otelPS); err != nil {
		s.T().Logf("windows ddot procmgr: otel-config bootstrap failed: %v", err)
		return
	}

	fleetPolicies := filepath.Join(configRoot, "Installer", "managed", "datadog-agent", "stable")
	mkdirPS := fmt.Sprintf(
		`powershell -NoProfile -Command "New-Item -ItemType Directory -Force -LiteralPath '%s' | Out-Null"`,
		psEscapeSingleQuotedPath(fleetPolicies),
	)
	host.MustExecute(mkdirPS)

	datadogYAML := filepath.Join(configRoot, "datadog.yaml")
	appendOtel := "\notelcollector:\n  enabled: true\n"
	b64AppendOtel := base64.StdEncoding.EncodeToString([]byte(appendOtel))
	enableOtelPS := fmt.Sprintf(
		`powershell -NoProfile -Command "$dy='%s'; if (-not (Test-Path -LiteralPath $dy)) { exit 0 }; if (-not (Select-String -LiteralPath $dy -Pattern 'otelcollector:' -Quiet)) { $a=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); Add-Content -LiteralPath $dy -Value $a -Encoding utf8 }"`,
		psEscapeSingleQuotedPath(datadogYAML),
		b64AppendOtel,
	)
	host.MustExecute(enableOtelPS)

	yamlPath := filepath.Join(installPath, "processes.d", "datadog-agent-ddot.yaml")
	yamlBody := windowsDDOTProcmgrYAMLContent(installPath, configRoot, fleetPolicies)
	b64 := base64.StdEncoding.EncodeToString([]byte(yamlBody))
	writePS := fmt.Sprintf(
		`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; $b=[Convert]::FromBase64String('%s'); [IO.File]::WriteAllBytes('%s', $b)"`,
		b64,
		psEscapeSingleQuotedPath(yamlPath),
	)
	if _, err := host.Execute(writePS); err != nil {
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

func psEscapeSingleQuotedPath(p string) string {
	return strings.ReplaceAll(p, `'`, `''`)
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
		restorePS := fmt.Sprintf(
			`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $c=$c.Replace('%s','%s'); $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)"`,
			psEscapeSingleQuotedPath(yamlPath),
			psEscapeSingleQuotedPath(windowsDDOTDescE2ELine),
			psEscapeSingleQuotedPath(windowsDDOTDescOriginalLine),
		)
		_, _ = s.Env().RemoteHost.Execute(restorePS)
		_, _ = s.Env().RemoteHost.Execute(s.platform.cliCmd("reload"))
	})

	applyPS := fmt.Sprintf(
		`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; $p='%s'; $c=[IO.File]::ReadAllText($p); $c=$c.Replace('%s','%s'); $enc=New-Object System.Text.UTF8Encoding $false; [IO.File]::WriteAllText($p,$c,$enc)"`,
		psEscapeSingleQuotedPath(yamlPath),
		psEscapeSingleQuotedPath(windowsDDOTDescOriginalLine),
		psEscapeSingleQuotedPath(windowsDDOTDescE2ELine),
	)
	s.Env().RemoteHost.MustExecute(applyPS)

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
