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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

// testContext adapts *testing.T to common.Context for agent client initialization.
type testContext struct {
	t *testing.T
}

func (c *testContext) T() *testing.T         { return c.t }
func (c *testContext) SessionOutputDir() string { return "" }

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
func Install(t *testing.T, env *environments.Host, opts ...agentparams.Option) {
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
func InstallOnHost(t *testing.T, host *components.RemoteHost, fakeIntake *components.FakeIntake, opts ...agentparams.Option) *components.RemoteHostAgent {
	t.Helper()
	require.NotNil(t, host, "hostagent.InstallOnHost: host is nil, infrastructure must be provisioned first")

	// Parse options to extract version info for install and config for later
	p := &agentparams.Params{
		Integrations: make(map[string]*agentparams.FileDefinition),
		Files:        make(map[string]*agentparams.FileDefinition),
	}
	for _, opt := range opts {
		require.NoError(t, opt(p))
	}

	// Read version defaults from runner profile (same source as Pulumi config)
	applyVersionDefaults(t, p)

	// Get API key for the install script
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(t, err, "failed to get API key")
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

	err = agentComp.Init(&testContext{t: t})
	require.NoError(t, err, "failed to initialize agent client")

	// Wire cross-component references for Configure
	agentComp.SetComponents(host, fakeIntake)

	// Set baseline options and apply initial configuration
	agentComp.SetBaseOptions(opts...)
	agentComp.Configure(t)

	return agentComp
}

// applyVersionDefaults reads version-related parameters from the runner
// profile and applies them to the Params if not already set by user options.
func applyVersionDefaults(t *testing.T, p *agentparams.Params) {
	t.Helper()
	profile := runner.GetProfile()

	// Default major version
	if p.Version.Major == "" {
		major, err := profile.ParamStore().GetWithDefault(parameters.MajorVersion, "7")
		require.NoError(t, err)
		p.Version.Major = major
	}

	// Pipeline ID overrides version
	if p.Version.PipelineID == "" && p.Version.Minor == "" && p.Version.LocalPath == "" {
		pipelineID, err := profile.ParamStore().GetWithDefault(parameters.PipelineID, "")
		require.NoError(t, err)
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
		require.NoError(t, err)
		if fips == "true" {
			p.Version.Flavor = agentparams.FIPSFlavor
		} else {
			p.Version.Flavor = agentparams.DefaultFlavor
		}
	}
}

// installAgent runs the appropriate install command via SSH based on the OS.
func installAgent(t *testing.T, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
	t.Helper()

	switch host.OSFamily {
	case oscomp.LinuxFamily:
		installLinuxAgent(t, host, version, apiKey)
	case oscomp.WindowsFamily:
		installWindowsAgent(t, host, version, apiKey)
	case oscomp.MacOSFamily:
		installMacOSAgent(t, host, version, apiKey)
	default:
		require.Fail(t, "unsupported OS family: %v", host.OSFamily)
	}
}

// installLinuxAgent installs the agent on Linux via the official install script.
// Mirrors the logic in components/datadog/agent/host_linuxos.go:getInstallCommand.
func installLinuxAgent(t *testing.T, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
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

// installWindowsAgent installs the agent on Windows via MSI.
// Mirrors the logic in components/datadog/agent/host_windowsos.go.
func installWindowsAgent(t *testing.T, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
	t.Helper()
	// TODO: Implement Windows agent install via SSH
	// This requires downloading the MSI and running msiexec
	require.Fail(t, "hostagent.Install: Windows agent install via SSH not yet implemented, use the Pulumi provisioner")
}

// installMacOSAgent installs the agent on macOS.
// Mirrors the logic in components/datadog/agent/host_macos.go.
func installMacOSAgent(t *testing.T, host *components.RemoteHost, version agentparams.PackageVersion, apiKey string) {
	t.Helper()
	// TODO: Implement macOS agent install via SSH
	require.Fail(t, "hostagent.Install: macOS agent install via SSH not yet implemented, use the Pulumi provisioner")
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
