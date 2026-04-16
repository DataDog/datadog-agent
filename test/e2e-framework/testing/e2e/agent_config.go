// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// SetAgentConfig reconfigures the agent on a provisioned host without
// re-running Pulumi. It writes config files via SSH and restarts the agent.
//
// The function builds the full datadog.yaml by merging the caller's options
// with framework defaults (API key, fakeintake intake URLs). This ensures
// the agent keeps working with fakeintake after a config change.
//
// Usage:
//
//	e2e.SetAgentConfig(s.T(), s.Env(),
//	    agentparams.WithAgentConfig("log_level: info"),
//	)
func SetAgentConfig(t *testing.T, env *environments.Host, opts ...agentparams.Option) {
	t.Helper()

	p := &agentparams.Params{
		Integrations: make(map[string]*agentparams.FileDefinition),
		Files:        make(map[string]*agentparams.FileDefinition),
	}
	for _, opt := range opts {
		require.NoError(t, opt(p))
	}

	host := env.RemoteHost
	configFolder, err := host.GetAgentConfigFolder()
	require.NoError(t, err, "failed to get agent config folder")

	// Build the full datadog.yaml with framework defaults merged in
	fullConfig := buildCoreAgentConfig(t, env, p)
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
	require.NoError(t, env.Agent.Client.Restart(), "failed to restart agent")
	restartSubAgentServices(t, host, p)
	require.Eventually(t, env.Agent.Client.IsReady, 2*time.Minute, 5*time.Second, "agent not ready after config change")
}

// buildCoreAgentConfig assembles the full datadog.yaml content by merging the
// user-provided AgentConfig with framework defaults:
//   - Fakeintake intake URLs (if a fakeintake is present in the environment)
//   - API key (from the runner's secret store)
//   - Any extra config from WithLogs, WithTags, WithHostname options
//
// This mirrors the merge logic in the Pulumi agent component
// (host.go:updateCoreAgentConfig) but uses resolved values instead of
// Pulumi outputs.
func buildCoreAgentConfig(t *testing.T, env *environments.Host, p *agentparams.Params) string {
	t.Helper()

	config := p.AgentConfig

	// Merge fakeintake intake URLs if a fakeintake is present
	if env.FakeIntake != nil && env.FakeIntake.URL != "" {
		intakeConfig := fakeintakeConfigYAML(env.FakeIntake.Scheme, env.FakeIntake.Host, env.FakeIntake.Port)
		config = mergeYAML(t, config, intakeConfig)
	}

	// Merge extra agent config (WithLogs, WithTags, WithHostname, etc.)
	// These are pulumi.StringInput but for the options that produce plain
	// strings (WithLogs, WithTags, WithHostname), the underlying value is
	// a pulumi.String which implements fmt.Stringer.
	for _, extra := range p.ExtraAgentConfig {
		config = mergeYAML(t, config, fmt.Sprintf("%s", extra))
	}

	// Merge API key
	if !p.SkipAPIKeyInConfig {
		apiKey := getAPIKey(t)
		config = mergeYAML(t, config, fmt.Sprintf("api_key: %s", apiKey))
	}

	return config
}

// fakeintakeConfigYAML generates the YAML config that points all agent
// forwarders to a fakeintake instance. This is the non-Pulumi equivalent
// of agentparams.withIntakeHostname.
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

// getAPIKey retrieves the Datadog API key from the runner's secret store.
func getAPIKey(t *testing.T) string {
	t.Helper()
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(t, err, "failed to get API key from secret store")
	return strings.TrimSpace(apiKey)
}

// mergeYAML merges two YAML strings, with the new values taking precedence.
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

// restartSubAgentServices restarts sub-agent services whose configs changed.
//
// On Linux, system-probe and security-agent are separate services;
// restarting datadog-agent does NOT restart them. The init system is
// auto-detected (systemctl, initctl, or service command).
//
// On Windows, all agents run under a single service managed by agent.exe,
// so no extra restarts are needed.
//
// On macOS, sub-agents are managed by the main launchd service.
func restartSubAgentServices(t *testing.T, host *components.RemoteHost, p *agentparams.Params) {
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

// restartLinuxService restarts a service on Linux, auto-detecting the init
// system. Errors are ignored since the service may not be installed.
func restartLinuxService(host *components.RemoteHost, service string) {
	if _, err := host.Execute("command -v systemctl"); err == nil {
		_, _ = host.Execute(fmt.Sprintf("sudo systemctl restart %s", service))
		return
	}
	if _, err := host.Execute("command -v /sbin/initctl"); err == nil {
		// upstart: stop then start, since restart fails if not running
		_, _ = host.Execute(fmt.Sprintf("sudo /sbin/initctl stop %s; sudo /sbin/initctl start %s", service, service))
		return
	}
	// Fallback to generic service command
	_, _ = host.Execute(fmt.Sprintf("sudo service %s restart", service))
}

// writeConfigFile writes a config file to the host using elevated privileges.
func writeConfigFile(t *testing.T, host *components.RemoteHost, fullPath string, content string) {
	t.Helper()
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		host.MustExecute(fmt.Sprintf("Set-Content -Path '%s' -Value @\"\n%s\n\"@", fullPath, content))
	default:
		host.MustExecute(fmt.Sprintf("sudo tee %s > /dev/null << 'AGENTCONFIGEOF'\n%s\nAGENTCONFIGEOF", fullPath, content))
	}
}

// writeFileDefinition writes a file and applies permissions from a FileDefinition.
func writeFileDefinition(t *testing.T, host *components.RemoteHost, fullPath string, fileDef *agentparams.FileDefinition) {
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

// mkdirPrivileged creates a directory with elevated privileges.
func mkdirPrivileged(t *testing.T, host *components.RemoteHost, dir string) {
	t.Helper()
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		host.MustExecute(fmt.Sprintf("New-Item -ItemType Directory -Force -Path '%s'", dir))
	default:
		host.MustExecute(fmt.Sprintf("sudo mkdir -p %s", dir))
	}
}

// dirPath returns the parent directory of a file path.
func dirPath(host *components.RemoteHost, filePath string) string {
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
