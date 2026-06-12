// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostagent provides functions to install and configure the Datadog
// Agent on a remote host via SSH, without relying on Pulumi.
package hostagent

import (
	"fmt"
	"strings"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/internal/agenturl"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

// Install installs the Datadog Agent on a remote host via SSH, configures it,
// and starts it. It populates env.Agent with the initialized agent component.
//
// The agent options are split into two phases:
//   - Install phase: version/flavor options determine which package to install
//   - Configure phase: config options (WithAgentConfig, WithIntegration, etc.)
//     are written via SSH after installation
//
// Usage in SetupSuite:
//
//	hostagent.Install(s.T(), s.Env(),
//	    agentparams.WithAgentConfig("log_level: debug"),
//	    agentparams.WithLogs(),
//	)
func Install(t common.Context, env *environments.Host, opts ...agentparams.Option) {
	t.Helper()
	env.Agent = InstallOnHost(t, env.RemoteHost, env.FakeIntake, opts...)
}

// InstallOnWindowsHost installs the Datadog Agent on a Windows host environment.
// It is the Windows-specific counterpart to Install for environments.WindowsHost.
func InstallOnWindowsHost(t common.Context, env *environments.WindowsHost, opts ...agentparams.Option) {
	t.Helper()
	env.Agent = InstallOnHost(t, env.RemoteHost, env.FakeIntake, opts...)
}

// InstallOnHost installs the Datadog Agent on the given remote host via SSH and
// returns the configured agent component. This is the building block used by
// Install for the standard environments.Host environment, but can also be used
// directly with custom environment types.
//
// The fakeintake parameter is optional — pass nil if no fakeintake is provisioned.
//
// Usage with a custom environment:
//
//	v.Env().Agent = hostagent.InstallOnHost(v.T(), v.Env().RemoteHost, v.Env().Fakeintake,
//	    agentparams.WithLogs(),
//	    agentparams.WithIntegration("nginx.d", nginxConfig),
//	)
func InstallOnHost(t common.Context, host *components.RemoteHost, fakeIntake *components.FakeIntake, opts ...agentparams.Option) *components.RemoteHostAgent {
	t.Helper()
	rt := common.RequireT{Context: t}
	require.NotNil(rt, host, "hostagent.InstallOnHost: host is nil, infrastructure must be provisioned first")

	// Parse options to extract version info for install and config for later
	p := &agentparams.Params{
		Integrations: make(map[string]*agentparams.FileDefinition),
		Files:        make(map[string]*agentparams.FileDefinition),
	}
	for _, opt := range opts {
		require.NoError(rt, opt(p))
	}

	// Read version defaults from runner profile (same source as Pulumi config)
	applyVersionDefaults(t, p)

	// Get API key for the install script
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(rt, err, "failed to get API key")
	apiKey = strings.TrimSpace(apiKey)

	// Run the install script via SSH
	installAgent(t, host, p.Version, apiKey)

	// Create and initialize the agent component.
	// Don't wait for agent ready — it hasn't been started yet.
	// Configure will start it and wait.
	agentComp := &components.RemoteHostAgent{
		ClientOptions: []agentclientparams.Option{agentclientparams.WithSkipWaitForAgentReady()},
	}
	agentComp.HostAgentOutput.Host = host.HostOutput
	agentComp.HostAgentOutput.FIPSEnabled = (p.Version.Flavor == agentparams.FIPSFlavor)

	err = agentComp.Init(t)
	require.NoError(rt, err, "failed to initialize agent client")

	// Wire cross-component references for Configure
	agentComp.SetComponents(host, fakeIntake)

	// Set baseline options and apply initial configuration
	agentComp.SetBaseOptions(opts...)
	agentComp.Configure(t)

	return agentComp
}

// applyVersionDefaults reads version-related parameters from the runner
// profile and applies them to the Params if not already set by user options.
func applyVersionDefaults(t common.Context, p *agentparams.Params) {
	t.Helper()
	rt := common.RequireT{Context: t}
	profile := runner.GetProfile()

	// Default major version
	if p.Version.Major == "" {
		major, err := profile.ParamStore().GetWithDefault(parameters.MajorVersion, "7")
		require.NoError(rt, err)
		p.Version.Major = major
	}

	// Pipeline ID overrides version
	if p.Version.PipelineID == "" && p.Version.Minor == "" && p.Version.LocalPath == "" {
		pipelineID, err := profile.ParamStore().GetWithDefault(parameters.PipelineID, "")
		require.NoError(rt, err)
		if pipelineID != "" {
			p.Version.PipelineID = pipelineID
		}
	}

	// Default channel
	if p.Version.Channel == "" {
		if p.Version.PipelineID != "" || p.Version.Minor == "" {
			p.Version.Channel = agentparams.NightlyChannel
		} else {
			p.Version.Channel = agentparams.StableChannel
		}
	}

	// Default flavor
	if p.Version.Flavor == "" {
		fips, err := profile.ParamStore().GetWithDefault(parameters.FIPS, "false")
		require.NoError(rt, err)
		if fips == "true" {
			p.Version.Flavor = agentparams.FIPSFlavor
		} else {
			p.Version.Flavor = agentparams.DefaultFlavor
		}
	}
}

// installAgent runs the appropriate install command via SSH based on the OS.
func installAgent(t common.Context, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
	t.Helper()

	switch host.OSFamily {
	case oscomp.LinuxFamily:
		installLinuxAgent(t, host, version, apiKey)
	case oscomp.WindowsFamily:
		installWindowsAgent(t, host, version, apiKey)
	case oscomp.MacOSFamily:
		installMacOSAgent(t, host, version, apiKey)
	default:
		require.Fail(common.RequireT{Context: t}, "unsupported OS family: %v", host.OSFamily)
	}
}

// installLinuxAgent installs the agent on Linux via the official install script.
// Mirrors the logic in components/datadog/agent/host_linuxos.go:getInstallCommand.
func installLinuxAgent(t common.Context, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
	t.Helper()

	var envVars []string

	if version.PipelineID != "" {
		envVars = append(envVars,
			fmt.Sprintf("TESTING_APT_URL=apttesting.datad0g.com/datadog-agent/pipeline-%s-a%s", version.PipelineID, version.Major),
			fmt.Sprintf(`TESTING_APT_REPO_VERSION="stable-%s %s"`, detectArch(host), version.Major),
			"TESTING_YUM_URL=yumtesting.datad0g.com",
			fmt.Sprintf("TESTING_YUM_VERSION_PATH=testing/pipeline-%s-a%s/%s", version.PipelineID, version.Major, version.Major),
			"TESTING_KEYS_URL=apttesting.datad0g.com/test-keys",
		)
	} else {
		envVars = append(envVars, fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%s", version.Major))
		if version.Minor != "" {
			envVars = append(envVars, fmt.Sprintf("DD_AGENT_MINOR_VERSION=%s", version.Minor))
		}
		if version.Channel != "" && version.Channel != agentparams.StableChannel {
			envVars = append(envVars, "REPO_URL=datad0g.com")
			envVars = append(envVars, fmt.Sprintf("DD_AGENT_DIST_CHANNEL=%s", version.Channel))
		}
	}

	if version.Flavor != "" {
		envVars = append(envVars, fmt.Sprintf("DD_AGENT_FLAVOR=%s", version.Flavor))
	}

	envStr := strings.Join(envVars, " ")
	scriptName := fmt.Sprintf("install_script_agent%s.sh", version.Major)

	cmd := fmt.Sprintf(
		`for i in 1 2 3 4 5; do curl -fsSL https://s3.amazonaws.com/dd-agent/scripts/%s -o install-script.sh && break || sleep $((2**$i)); done && `+
			`for i in 1 2 3; do DD_API_KEY=%s %s DD_INSTALL_ONLY=true bash install-script.sh && exit 0 || sleep $((2**$i)); done; exit 1`,
		scriptName, apiKey, envStr,
	)

	host.MustExecute(cmd)
}

// installWindowsAgent installs the Datadog Agent on Windows via PowerShell over SSH.
// Mirrors the logic in components/datadog/agent/host_windowsos.go:getInstallCommand.
func installWindowsAgent(t common.Context, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
	t.Helper()

	if version.Flavor == "" {
		version.Flavor = agentparams.DefaultFlavor
	}

	msiURL, err := agenturl.WindowsMSI(version)
	require.NoError(common.RequireT{Context: t}, err, "failed to resolve Windows MSI URL")

	localFilename := `C:\datadog-agent.msi`
	logFile := `C:\datadog-agent-install.log`

	var cmd string

	// Enable FIPS policy before install if requested.
	if version.Flavor == agentparams.FIPSFlavor {
		cmd += `Set-ItemProperty -Path 'HKLM:\System\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy' -Name 'Enabled' -Value 1 -Type DWORD; `
	}

	// Download the MSI with retries.
	cmd += fmt.Sprintf(`
$ProgressPreference = 'SilentlyContinue';
$ErrorActionPreference = 'Stop';
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
for ($i=0; $i -lt 3; $i++) {
    try { (New-Object Net.WebClient).DownloadFile('%s','%s'); break } catch { if ($i -eq 2) { throw } }
};`, msiURL, localFilename)

	// Run msiexec silently.
	cmd += fmt.Sprintf(`
$exitCode = (Start-Process -Wait msiexec -PassThru -ArgumentList '/qn /i %s APIKEY=%s /log %s').ExitCode;
Get-Content %s;
if ($exitCode -ne 0) { exit $exitCode }`,
		localFilename, apiKey, logFile, logFile)

	host.MustExecute(cmd)
}

// installMacOSAgent installs the Datadog Agent on macOS via the official install script over SSH.
// Mirrors the logic in components/datadog/agent/host_macos.go:getInstallCommand.
func installMacOSAgent(t common.Context, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
	t.Helper()

	var exports []string
	if version.Major != "" {
		exports = append(exports, fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%s", version.Major))
	}
	if version.Minor != "" {
		exports = append(exports, fmt.Sprintf("DD_AGENT_MINOR_VERSION=%s", version.Minor))
	}
	if version.PipelineID != "" {
		exports = append(exports, fmt.Sprintf("DD_REPO_URL=https://dd-agent-macostesting.s3.amazonaws.com/ci/datadog-agent/pipeline-%s-%s",
			version.PipelineID, host.Architecture))
	}

	envStr := strings.Join(exports, " ")
	cmd := fmt.Sprintf(
		`for i in 1 2 3 4 5; do curl -fsSL https://install.datadoghq.com/scripts/install_mac_os.sh -o install-script.sh && break || sleep $((2**$i)); done && `+
			`for i in 1 2 3; do DD_API_KEY=%s %s DD_INSTALL_ONLY=true bash install-script.sh && exit 0 || sleep $((2**$i)); done; exit 1`,
		apiKey, envStr,
	)

	host.MustExecute(cmd)
}

// detectArch returns the architecture string for the current host.
func detectArch(host *components.RemoteHost) string {
	arch, err := host.Execute("dpkg --print-architecture 2>/dev/null || uname -m")
	if err != nil {
		return "amd64"
	}
	arch = strings.TrimSpace(arch)
	switch arch {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return arch
	}
}
