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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	e2ecomponents "github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
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

	// Static suite fixtures live under processes.d as YAML (see TestProcmgrSmokeWindowsSuite).
	// Runtime create is for per-test definitions (unique names, RPC auth checks, etc.).

	winMissingBinaryConfig = `command: C:\nonexistent\binary.exe
condition_path_exists: C:\nonexistent\binary.exe
auto_start: true
restart: never
description: should not start
`

	// Description line from fleet/embedded DDOT template.
	windowsDDOTDescOriginalLine = "description: Datadog Distribution of OpenTelemetry Collector"
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
					agentparams.WithFile(winConfigDir+"/missing-binary.yaml", winMissingBinaryConfig, true),
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

	if _, err := host.Execute(`powershell -Command "Restart-Service dd-procmgr-service -Force"`); err != nil {
		s.T().Logf("windows ddot procmgr: procmgr service restart failed: %v", err)
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
