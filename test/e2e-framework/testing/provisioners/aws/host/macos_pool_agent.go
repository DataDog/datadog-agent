// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awshost

import (
	"encoding/base64"
	"fmt"
	"path"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	e2eclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

// macOSAgentConfigFolder mirrors agentMacOSManager.getAgentConfigFolder
// (components/datadog/agent/host_macos.go).
const macOSAgentConfigFolder = "/opt/datadog-agent/etc"

// provisionMacOSPoolAgent installs the Agent on host over SSH, sequentially, mirroring
// agentMacOSManager's install-script flow (components/datadog/agent/host_macos.go and
// components/datadog/agent/host.go) but without a *pulumi.Context: commands run directly
// through host.Execute/WriteFile instead of being scheduled as Pulumi command.Command
// resources. Only the install-script path is supported; a local .dmg/.pkg (LocalPath) is
// not, matching the framework-wide constraint of this Pulumi-free pool provisioner.
func provisionMacOSPoolAgent(host *e2eclient.Host, hostOutput remote.HostOutput, apiKey string, agentOptions []agentparams.Option, fakeIntakeExtraConfig string) (*agent.HostAgentOutput, error) {
	params, err := resolveMacOSPoolAgentParams(agentOptions)
	if err != nil {
		return nil, err
	}
	if params.Version.LocalPath != "" {
		return nil, fmt.Errorf("installing the macOS pool agent from a local package is not supported")
	}

	if _, err := host.Execute(macOSInstallScriptCommand(params.Version, apiKey)); err != nil {
		return nil, fmt.Errorf("failed to run macOS agent install script: %w", err)
	}

	if err := writeMacOSAgentConfig(host, "datadog.yaml", params.AgentConfig, params.ExtraAgentConfig, fakeIntakeExtraConfig, apiKey, params.SkipAPIKeyInConfig); err != nil {
		return nil, err
	}
	for _, cfg := range []struct{ path, content string }{
		{"system-probe.yaml", params.SystemProbeConfig},
		{"security-agent.yaml", params.SecurityAgentConfig},
	} {
		if cfg.content == "" {
			continue
		}
		if err := writeMacOSAgentConfig(host, cfg.path, cfg.content, nil, "", "", true); err != nil {
			return nil, err
		}
	}

	for confPath, def := range params.Integrations {
		if err := writeMacOSAgentFile(host, path.Join(macOSAgentConfigFolder, confPath), def); err != nil {
			return nil, fmt.Errorf("failed to write integration config %s: %w", confPath, err)
		}
	}
	for absPath, def := range params.Files {
		if err := writeMacOSAgentFile(host, absPath, def); err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", absPath, err)
		}
	}

	if _, err := host.Execute("sudo launchctl kickstart -k system/com.datadoghq.agent"); err != nil {
		return nil, fmt.Errorf("failed to restart macOS agent service: %w", err)
	}

	return &agent.HostAgentOutput{
		Host:        hostOutput,
		FIPSEnabled: params.Version.Flavor == agentparams.FIPSFlavor,
	}, nil
}

// resolveMacOSPoolAgentParams applies agentOptions over sane, Pulumi-free defaults
// mirroring agentparams.NewParams' defaults (WithLatestNightly, DefaultFlavor,
// DefaultMajorVersion) — NewParams itself cannot be used here since it requires a
// config.Env, whose PipelineID/AgentAPIKey/etc. accessors read live Pulumi stack config.
func resolveMacOSPoolAgentParams(agentOptions []agentparams.Option) (*agentparams.Params, error) {
	params := &agentparams.Params{
		Integrations: make(map[string]*agentparams.FileDefinition),
		Files:        make(map[string]*agentparams.FileDefinition),
	}
	defaults := []agentparams.Option{
		agentparams.WithLatestNightly(),
		agentparams.WithMajorVersion(config.DefaultMajorVersion),
		agentparams.WithFlavor(agentparams.DefaultFlavor),
	}
	return common.ApplyOption(params, append(defaults, agentOptions...))
}

// macOSInstallScriptCommand replicates agentMacOSManager.getInstallCommand
// (components/datadog/agent/host_macos.go) with a plain apiKey string in place of a
// pulumi.StringInput.
func macOSInstallScriptCommand(version agentparams.PackageVersion, apiKey string) string {
	exports := []string{}
	if version.Major != "" {
		exports = append(exports, fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%s", version.Major))
	}
	if version.Minor != "" {
		exports = append(exports, fmt.Sprintf("DD_AGENT_MINOR_VERSION=%s", version.Minor))
	}
	if version.PipelineID != "" {
		exports = append(exports, fmt.Sprintf("DD_REPO_URL=https://dd-agent-macostesting.s3.amazonaws.com/ci/datadog-agent/pipeline-%s", version.PipelineID))
	}

	env := ""
	for _, e := range exports {
		env += e + " "
	}

	return fmt.Sprintf(`for i in 1 2 3 4 5; do curl -fsSL https://install.datadoghq.com/scripts/install_mac_os.sh -o install-script.sh && break || sleep $((2**$i)); done && for i in 1 2 3; do DD_API_KEY=%s %sDD_INSTALL_ONLY=true bash install-script.sh && exit 0 || sleep $((2**$i)); done; exit 1`, apiKey, env)
}

// writeMacOSAgentConfig merges baseConfig with extraAgentConfig (converted from
// pulumi.StringInput, which must resolve to a plain pulumi.String literal since there is
// no pulumi engine here to evaluate Outputs), fakeIntakeExtraConfig, and an api_key line
// (unless skipAPIKey), then writes the result to configPath under the Agent's config
// folder.
func writeMacOSAgentConfig(host *e2eclient.Host, configPath, baseConfig string, extraAgentConfig []pulumi.StringInput, fakeIntakeExtraConfig, apiKey string, skipAPIKey bool) error {
	merged := baseConfig
	for _, extra := range extraAgentConfig {
		str, ok := extra.(pulumi.String)
		if !ok {
			return fmt.Errorf("extra agent config for %s must be a plain pulumi.String literal in the Pulumi-free macOS pool path, got %T", configPath, extra)
		}
		var err error
		merged, err = utils.MergeYAMLWithSlices(merged, string(str))
		if err != nil {
			return fmt.Errorf("failed to merge extra agent config into %s: %w", configPath, err)
		}
	}

	var err error
	if fakeIntakeExtraConfig != "" {
		merged, err = utils.MergeYAMLWithSlices(merged, fakeIntakeExtraConfig)
		if err != nil {
			return fmt.Errorf("failed to merge fakeintake config into %s: %w", configPath, err)
		}
	}
	if !skipAPIKey {
		merged, err = utils.MergeYAMLWithSlices(merged, fmt.Sprintf("api_key: %s", apiKey))
		if err != nil {
			return fmt.Errorf("failed to merge api_key into %s: %w", configPath, err)
		}
	}

	fullPath := path.Join(macOSAgentConfigFolder, configPath)
	if _, err := host.Execute(fmt.Sprintf("mkdir -p %s", macOSAgentConfigFolder)); err != nil {
		return fmt.Errorf("failed to create agent config folder: %w", err)
	}
	return writeMacOSFileWithSudo(host, fullPath, merged)
}

// writeMacOSAgentFile writes def to fullPath, creating parent directories and applying
// permissions as agentMacOSManager.writeFileDefinition
// (components/datadog/agent/host.go) does through Pulumi commands.
func writeMacOSAgentFile(host *e2eclient.Host, fullPath string, def *agentparams.FileDefinition) error {
	if _, err := host.Execute(fmt.Sprintf("mkdir -p %s", path.Dir(fullPath))); err != nil {
		return fmt.Errorf("failed to create parent directory for %s: %w", fullPath, err)
	}

	if err := writeMacOSFileWithSudo(host, fullPath, def.Content); err != nil {
		return err
	}

	if permsValue, found := def.Permissions.Get(); found {
		if cmd := permsValue.SetupPermissionsCommand(fullPath); cmd != "" {
			if _, err := host.Execute(cmd); err != nil {
				return fmt.Errorf("failed to set permissions on %s: %w", fullPath, err)
			}
		}
	}
	return nil
}

// macOSFakeIntakeExtraConfig renders the same intake-hostname and Remote Config settings
// as agentparams.WithFakeintake (components/datadog/agentparams/params.go), using plain
// strings from fi instead of pulumi.StringOutput/ApplyT, since there is no pulumi engine
// here to evaluate Outputs.
func macOSFakeIntakeExtraConfig(fi *fakeintake.FakeintakeOutput) (string, error) {
	rootJSON, err := fakeintake.RCRootJSON()
	if err != nil {
		return "", fmt.Errorf("build fakeintake rc root json: %w", err)
	}

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
network_devices.config_management.forwarder.logs_dd_url: %[1]s:%[2]d
network_devices.config_management.forwarder.logs_no_ssl: true
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
config_files_discovery.forwarder.logs_dd_url: %[1]s:%[2]d
config_files_discovery.forwarder.logs_no_ssl: true
software_inventory.forwarder.logs_dd_url: %[1]s:%[2]d
software_inventory.forwarder.logs_no_ssl: true
data_streams.forwarder.logs_dd_url: %[1]s:%[2]d
data_streams.forwarder.logs_no_ssl: true
event_management.forwarder.logs_dd_url: %[1]s:%[2]d
event_management.forwarder.logs_no_ssl: true
agent_telemetry.logs_dd_url: %[1]s:%[2]d
agent_telemetry.logs_no_ssl: true
agent_telemetry.use_compression: false
compliance_config.endpoints.logs_dd_url: %[1]s:%[2]d
compliance_config.endpoints.logs_no_ssl: true
compliance_config.endpoints.force_use_http: true
remote_configuration.enabled: true
remote_configuration.rc_dd_url: %[4]s
remote_configuration.no_tls: true
remote_configuration.refresh_interval: 5s
remote_configuration.config_root: '%[5]s'
remote_configuration.director_root: '%[5]s'
`, fi.Host, fi.Port, fi.Scheme, fi.URL, rootJSON), nil
}

// writeMacOSFileWithSudo writes content to fullPath via a base64-encoded sudo tee, since
// the SSH login user (ec2-user) does not own /opt/datadog-agent and host.WriteFile's sftp
// client is unprivileged.
func writeMacOSFileWithSudo(host *e2eclient.Host, fullPath, content string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	cmd := fmt.Sprintf("echo %s | base64 --decode | sudo tee %s > /dev/null", encoded, fullPath)
	if _, err := host.Execute(cmd); err != nil {
		return fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}
	return nil
}
