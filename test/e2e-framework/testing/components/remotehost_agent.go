// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

// RemoteHostAgent represents an Agent running directly on a Host
type RemoteHostAgent struct {
	agent.HostAgentOutput

	Client        agentclient.Agent
	ClientOptions []agentclientparams.Option

	// References to sibling components needed for Configure.
	// Set by SetComponents after all components are initialized.
	host       *RemoteHost
	fakeIntake *FakeIntake

	// baseOptions stores the agent options from the initial configuration.
	// Configure merges new options on top of these.
	baseOptions []agentparams.Option
}

var _ common.Initializable = (*RemoteHostAgent)(nil)

// Init is called by e2e test Suite after the component is provisioned.
func (a *RemoteHostAgent) Init(ctx common.Context) (err error) {
	a.Client, err = client.NewHostAgentClientWithParams(ctx, a.HostAgentOutput.Host, a.ClientOptions...)
	return err
}

// SetComponents stores references to sibling environment components
// that Configure needs (host for SSH, fakeintake for intake URLs).
// This should be called after all components are initialized.
func (a *RemoteHostAgent) SetComponents(host *RemoteHost, fakeIntake *FakeIntake) {
	a.host = host
	a.fakeIntake = fakeIntake
}

// Configure reconfigures the agent with new options, merging them on top
// of the baseline options from the initial installation/configuration.
//
// Options are merged by applying baseline first, then the new options.
// This means new options override baseline values for scalar fields
// (AgentConfig, SystemProbeConfig) and add to collection fields
// (Integrations, Files).
//
// Usage:
//
//	s.Env().Agent.Configure(s.T(),
//	    agentparams.WithAgentConfig("log_level: info"),
//	)
func (a *RemoteHostAgent) Configure(t *testing.T, opts ...agentparams.Option) {
	t.Helper()
	require.NotNil(t, a.host, "RemoteHostAgent.Configure: host not set, call SetComponents first")

	// Merge: apply baseline options first, then caller's overrides
	merged := make([]agentparams.Option, 0, len(a.baseOptions)+len(opts))
	merged = append(merged, a.baseOptions...)
	merged = append(merged, opts...)

	applyAgentConfig(t, a.host, a.fakeIntake, a.Client, merged)
}

// SetBaseOptions stores the baseline agent options. Configure merges new
// options on top of these. Call this after the initial agent configuration.
func (a *RemoteHostAgent) SetBaseOptions(opts ...agentparams.Option) {
	a.baseOptions = opts
}

// applyAgentConfig writes agent config files via SSH and restarts the agent.
// This is the core implementation shared by Configure and SetAgentConfig.
func applyAgentConfig(t *testing.T, host *RemoteHost, fakeIntake *FakeIntake, agentClient agentclient.Agent, opts []agentparams.Option) {
	t.Helper()

	p := &agentparams.Params{
		Integrations: make(map[string]*agentparams.FileDefinition),
		Files:        make(map[string]*agentparams.FileDefinition),
	}
	for _, opt := range opts {
		require.NoError(t, opt(p))
	}

	configFolder, err := host.GetAgentConfigFolder()
	require.NoError(t, err, "failed to get agent config folder")

	// Build the full datadog.yaml with framework defaults merged in
	fullConfig := buildCoreAgentConfig(t, host, fakeIntake, p)
	writeConfigFile(t, host, host.JoinPath(configFolder, "datadog.yaml"), fullConfig)

	// Write system-probe.yaml
	if p.SystemProbeConfig != "" {
		writeConfigFile(t, host, host.JoinPath(configFolder, "system-probe.yaml"), p.SystemProbeConfig)
	}

	// Write security-agent.yaml
	if p.SecurityAgentConfig != "" {
		writeConfigFile(t, host, host.JoinPath(configFolder, "security-agent.yaml"), p.SecurityAgentConfig)
	}

	// Write integration configs (conf.d/<name>/conf.yaml)
	for confPath, fileDef := range p.Integrations {
		fullPath := host.JoinPath(configFolder, confPath)
		mkdirPrivileged(t, host, dirPath(host, fullPath))
		writeFileDefinition(t, host, fullPath, fileDef)
	}

	// Write arbitrary files
	for filePath, fileDef := range p.Files {
		dir := dirPath(host, filePath)
		if fileDef.UseSudo {
			mkdirPrivileged(t, host, dir)
		} else {
			require.NoError(t, host.MkdirAll(dir), "failed to create directory %s", dir)
		}
		writeFileDefinition(t, host, filePath, fileDef)
	}

	// Restart the agent and wait for ready.
	require.NoError(t, agentClient.Restart(), "failed to restart agent")
	restartSubAgentServices(t, host, p)
	require.Eventually(t, agentClient.IsReady, 2*time.Minute, 5*time.Second, "agent not ready after config change")
}

// buildCoreAgentConfig assembles the full datadog.yaml content by merging the
// user-provided AgentConfig with framework defaults (fakeintake URLs, API key).
func buildCoreAgentConfig(t *testing.T, host *RemoteHost, fakeIntake *FakeIntake, p *agentparams.Params) string {
	t.Helper()

	config := p.AgentConfig

	// Merge fakeintake intake URLs if a fakeintake is present
	if fakeIntake != nil && fakeIntake.URL != "" {
		intakeConfig := fakeintakeConfigYAML(fakeIntake.Scheme, fakeIntake.Host, fakeIntake.Port)
		config = mergeYAML(t, config, intakeConfig)
	}

	// Merge extra agent config (WithLogs, WithTags, WithHostname,
	// WithIntakeHostname, WithFakeintake, etc.). Uses ExtraAgentConfigRaw
	// since ExtraAgentConfig contains pulumi.StringInput values that
	// can't be resolved outside a Pulumi context.
	for _, extra := range p.ExtraAgentConfigRaw {
		config = mergeYAML(t, config, extra)
	}

	// Merge API key
	if !p.SkipAPIKeyInConfig {
		apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
		require.NoError(t, err, "failed to get API key from secret store")
		config = mergeYAML(t, config, fmt.Sprintf("api_key: %s", strings.TrimSpace(apiKey)))
	}

	return config
}

// fakeintakeConfigYAML generates the YAML config that points all agent
// forwarders to a fakeintake instance.
func fakeintakeConfigYAML(scheme, host string, port uint32) string {
	return fmt.Sprintf(`dd_url: %[3]s://%[1]s:%[2]d
logs_config.logs_dd_url: %[1]s:%[2]d
logs_config.logs_no_ssl: true
logs_config.force_use_http: true
process_config.process_dd_url: %[3]s://%[1]s:%[2]d
apm_config.apm_dd_url: %[3]s://%[1]s:%[2]d
database_monitoring.metrics.logs_dd_url: %[1]s:%[2]d
database_monitoring.metrics.logs_no_ssl: true
database_monitoring.activity.logs_dd_url: %[1]s:%[2]d
database_monitoring.activity.logs_no_ssl: true
database_monitoring.samples.logs_dd_url: %[1]s:%[2]d
database_monitoring.samples.logs_no_ssl: true
network_devices.metadata.logs_dd_url: %[1]s:%[2]d
network_devices.metadata.logs_no_ssl: true
network_devices.snmp_traps.forwarder.logs_dd_url: %[1]s:%[2]d
network_devices.snmp_traps.forwarder.logs_no_ssl: true
network_devices.netflow.forwarder.logs_dd_url: %[1]s:%[2]d
network_devices.netflow.forwarder.logs_no_ssl: true
network_path.forwarder.logs_dd_url: %[1]s:%[2]d
network_path.forwarder.logs_no_ssl: true
network_config_management.forwarder.logs_dd_url: %[1]s:%[2]d
network_config_management.forwarder.logs_no_ssl: true
synthetics.forwarder.logs_dd_url: %[1]s:%[2]d
synthetics.forwarder.logs_no_ssl: true
container_lifecycle.logs_dd_url: %[1]s:%[2]d
container_lifecycle.logs_no_ssl: true
container_image.logs_dd_url: %[1]s:%[2]d
container_image.logs_no_ssl: true
sbom.logs_dd_url: %[1]s:%[2]d
sbom.logs_no_ssl: true
service_discovery.forwarder.logs_dd_url: %[1]s:%[2]d
service_discovery.forwarder.logs_no_ssl: true
software_inventory.forwarder.logs_dd_url: %[1]s:%[2]d
software_inventory.forwarder.logs_no_ssl: true
data_streams.forwarder.logs_dd_url: %[1]s:%[2]d
data_streams.forwarder.logs_no_ssl: true
event_management.forwarder.logs_dd_url: %[1]s:%[2]d
event_management.forwarder.logs_no_ssl: true
`, host, port, scheme)
}

func mergeYAML(t *testing.T, base, overlay string) string {
	t.Helper()
	if base == "" {
		return overlay
	}
	if overlay == "" {
		return base
	}
	merged, err := utils.MergeYAMLWithSlices(base, overlay)
	require.NoError(t, err, "failed to merge agent config YAML")
	return merged
}

func restartSubAgentServices(t *testing.T, host *RemoteHost, p *agentparams.Params) {
	t.Helper()

	switch host.OSFamily {
	case oscomp.LinuxFamily:
		services := make([]string, 0, 2)
		if p.SystemProbeConfig != "" {
			services = append(services, "datadog-agent-sysprobe")
		}
		if p.SecurityAgentConfig != "" {
			services = append(services, "datadog-agent-security")
		}
		for _, svc := range services {
			restartLinuxService(host, svc)
		}
	case oscomp.WindowsFamily:
		// On Windows, agent.exe restart-service handles all sub-agents.
	case oscomp.MacOSFamily:
		// On macOS, the main launchd service manages sub-agents.
	}
}

func restartLinuxService(host *RemoteHost, service string) {
	if _, err := host.Execute("command -v systemctl"); err == nil {
		_, _ = host.Execute(fmt.Sprintf("sudo systemctl restart %s", service))
		return
	}
	if _, err := host.Execute("command -v /sbin/initctl"); err == nil {
		_, _ = host.Execute(fmt.Sprintf("sudo /sbin/initctl stop %s; sudo /sbin/initctl start %s", service, service))
		return
	}
	_, _ = host.Execute(fmt.Sprintf("sudo service %s restart", service))
}

func writeConfigFile(t *testing.T, host *RemoteHost, fullPath string, content string) {
	t.Helper()
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		host.MustExecute(fmt.Sprintf("Set-Content -Path '%s' -Value @\"\n%s\n\"@", fullPath, content))
	default:
		host.MustExecute(fmt.Sprintf("sudo tee %s > /dev/null << 'AGENTCONFIGEOF'\n%s\nAGENTCONFIGEOF", fullPath, content))
	}
}

func writeFileDefinition(t *testing.T, host *RemoteHost, fullPath string, fileDef *agentparams.FileDefinition) {
	t.Helper()
	if fileDef.UseSudo {
		writeConfigFile(t, host, fullPath, fileDef.Content)
	} else {
		_, err := host.WriteFile(fullPath, []byte(fileDef.Content))
		require.NoError(t, err, "failed to write file %s", fullPath)
	}
	if perms, ok := fileDef.Permissions.Get(); ok {
		cmd := perms.SetupPermissionsCommand(fullPath)
		if cmd != "" {
			host.MustExecute(cmd)
		}
	}
}

func mkdirPrivileged(t *testing.T, host *RemoteHost, dir string) {
	t.Helper()
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		host.MustExecute(fmt.Sprintf("New-Item -ItemType Directory -Force -Path '%s'", dir))
	default:
		host.MustExecute(fmt.Sprintf("sudo mkdir -p %s", dir))
	}
}

func dirPath(host *RemoteHost, filePath string) string {
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		for i := len(filePath) - 1; i >= 0; i-- {
			if filePath[i] == '\\' || filePath[i] == '/' {
				return filePath[:i]
			}
		}
	default:
		for i := len(filePath) - 1; i >= 0; i-- {
			if filePath[i] == '/' {
				return filePath[:i]
			}
		}
	}
	return filePath
}
