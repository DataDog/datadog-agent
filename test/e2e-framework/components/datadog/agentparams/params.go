// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentparams

import (
	"fmt"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Params defines the parameters for the Agent installation.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithLatest]
//   - [WithVersion]
//   - [WithPipeline]
//   - [WithAgentConfig]
//   - [WithSystemProbeConfig]
//   - [WithSecurityAgentConfig]
//   - [WithIntegration]
//   - [WithFile]
//   - [WithTelemetry]
//   - [WithPulumiResourceOptions]
//   - [withIntakeHostname]
//   - [WithIntakeHostname]
//   - [WithFakeintake]
//   - [WithLogs]
//   - [WithAdditionalInstallParameters]
//   - [WithSkipAPIKeyInConfig]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis

type FileDefinition struct {
	Content     string
	UseSudo     bool
	Permissions option.Option[perms.FilePermissions]
}

type Params struct {
	Version             PackageVersion
	AgentConfig         string
	SystemProbeConfig   string
	SecurityAgentConfig string
	Integrations        map[string]*FileDefinition
	Files               map[string]*FileDefinition
	ExtraAgentConfig    []pulumi.StringInput
	ResourceOptions     []pulumi.ResourceOption
	// This is a list of additional installer flags that can be used to pass installer-specific
	// parameters like the MSI flags.
	AdditionalInstallParameters []string
	SkipAPIKeyInConfig          bool
}

type Option = func(*Params) error

func NewParams(env config.Env, options ...Option) (*Params, error) {
	p := &Params{
		Integrations: make(map[string]*FileDefinition),
		Files:        make(map[string]*FileDefinition),
	}
	defaultVersion := WithLatestNightly()
	defaultFlavor := WithFlavor(DefaultFlavor)
	if env.PipelineID() != "" {
		fmt.Printf("Pipeline ID: %s\n", env.PipelineID())
		defaultVersion = WithPipeline(env.PipelineID())
	}
	if env.AgentLocalPackage() != "" {
		defaultVersion = WithLocalPackage(env.AgentLocalPackage())
	}
	if env.AgentVersion() != "" {
		defaultVersion = WithVersion(env.AgentVersion())
	}

	if env.AgentFIPS() {
		defaultFlavor = WithFlavor(FIPSFlavor)
	}

	options = append([]Option{defaultFlavor}, options...)
	options = append([]Option{WithMajorVersion(env.MajorVersion())}, options...)
	options = append([]Option{defaultVersion}, options...)
	return common.ApplyOption(p, options)
}

// WithLatest uses the latest Agent 7 version in the stable channel.
func WithLatest() func(*Params) error {
	return func(p *Params) error {
		p.Version.Major = "7"
		p.Version.Channel = StableChannel
		return nil
	}
}

func WithLatestNightly() func(*Params) error {
	return func(p *Params) error {
		p.Version.Major = "7"
		p.Version.Channel = NightlyChannel
		return nil
	}
}

// WithVersion use a specific version of the Agent. For example: `6.39.0` or `7.41.0~rc.7-1`
func WithVersion(version string) func(*Params) error {
	return func(p *Params) error {
		v, err := parseVersion(version)
		if err != nil {
			return err
		}
		p.Version = v

		return nil
	}
}

// WithFlavor use a specific flavor of the Agent. For example: `datadog-fips-agent`
//
// See PackageFlavor https://github.com/DataDog/agent-release-management/blob/main/generator/const.py
func WithFlavor(flavor string) func(*Params) error {
	return func(p *Params) error {
		p.Version.Flavor = flavor
		return nil
	}
}

// WithLocalPackage use a local package of the Agent
func WithLocalPackage(path string) func(*Params) error {
	return func(p *Params) error {
		p.Version.LocalPath = path
		return nil
	}
}

// WithPipeline use a specific version of the Agent by pipeline id
func WithPipeline(pipelineID string) func(*Params) error {
	return func(p *Params) error {
		p.Version.PipelineID = pipelineID
		return nil
	}
}

// WithMajorVersion specify the major version of the Agent
func WithMajorVersion(majorVersion string) func(*Params) error {
	return func(p *Params) error {
		p.Version.Major = majorVersion
		return nil
	}
}

func parseVersion(s string) (PackageVersion, error) {
	version := PackageVersion{}

	prefix := "7."
	if strings.HasPrefix(s, prefix) {
		version.Major = "7"
	} else {
		prefix = "6."
		if strings.HasPrefix(s, prefix) {
			version.Major = "6"
		} else {
			return version, fmt.Errorf("invalid version of the Agent: %v. The Agent version should starts with `7.` or `6.`", s)
		}
	}
	version.Minor = strings.TrimPrefix(s, prefix)

	version.Channel = StableChannel
	if strings.Contains(s, "~") {
		version.Channel = BetaChannel
	}

	return version, nil
}

// WithAgentConfig sets the configuration of the Agent.
func WithAgentConfig(config string) func(*Params) error {
	return func(p *Params) error {
		p.AgentConfig = config
		return nil
	}
}

// WithSystemProbeConfig sets the configuration of system-probe.
func WithSystemProbeConfig(config string) func(*Params) error {
	return func(p *Params) error {
		p.SystemProbeConfig = config
		return nil
	}
}

// WithSecurityAgentConfig sets the configuration of the security-agent.
func WithSecurityAgentConfig(config string) func(*Params) error {
	return func(p *Params) error {
		p.SecurityAgentConfig = config
		return nil
	}
}

// WithIntegration adds the configuration for an integration.
func WithIntegration(folderName string, content string) func(*Params) error {
	return func(p *Params) error {
		confPath := path.Join("conf.d", folderName, "conf.yaml")
		p.Integrations[confPath] = &FileDefinition{
			Content: content,
			UseSudo: true,
		}
		return nil
	}
}

// WithFile adds a file with contents to the install at the given path. This should only be used when the agent needs to be restarted after writing the file.
func WithFile(absolutePath string, content string, useSudo bool) func(*Params) error {
	return WithFileWithPermissions(absolutePath, content, useSudo, option.None[perms.FilePermissions]())
}

// WithFileWithPermissions adds a file like WithFile but we can predefine the permissions of the file.
func WithFileWithPermissions(absolutePath string, content string, useSudo bool, perms option.Option[perms.FilePermissions]) func(*Params) error {
	return func(p *Params) error {
		p.Files[absolutePath] = &FileDefinition{
			Content:     content,
			UseSudo:     useSudo,
			Permissions: perms,
		}
		return nil
	}
}

// WithTelemetry enables the Agent telemetry go_expvar and openmetrics.
func WithTelemetry() func(*Params) error {
	return func(p *Params) error {
		config := `instances:
  - expvar_url: http://localhost:5000/debug/vars
    max_returned_metrics: 1000
    metrics:
      - path: ".*"
      - path: ".*/.*"
      - path: ".*/.*/.*"
`
		if err := WithIntegration("go_expvar.d", config)(p); err != nil {
			return err
		}

		config = `instances:
  - prometheus_url: http://localhost:5000/telemetry
    namespace: "datadog"
    metrics:
      - "*"
`
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.String("telemetry.enabled: true"))
		return WithIntegration("openmetrics.d", config)(p)
	}
}

func WithPulumiResourceOptions(resources ...pulumi.ResourceOption) func(*Params) error {
	return func(p *Params) error {
		p.ResourceOptions = append(p.ResourceOptions, resources...)
		return nil
	}
}

func withIntakeHostname(scheme pulumi.StringInput, hostname pulumi.StringInput, port pulumi.IntInput) func(*Params) error {
	return func(p *Params) error {
		extraConfig := pulumi.Sprintf(`dd_url: %[3]s://%[1]s:%[2]d
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
`, hostname, port, scheme)
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, extraConfig)
		return nil
	}
}

// WithIntakeName configures the agent to use the given hostname as intake.
//
// To use a fakeintake, see WithFakeintake.
//
// This option is overwritten by `WithFakeintake`.
func WithIntakeHostname(scheme string, hostname string) func(*Params) error {
	var port uint32
	if scheme == "http" {
		port = 80
	} else {
		port = 443
	}

	return withIntakeHostname(pulumi.String(scheme), pulumi.String(hostname), pulumi.Int(port))
}

// WithFakeintake installs the fake intake and configures the Agent to use it.
//
// This option is overwritten by `WithIntakeHostname`.
func WithFakeintake(fakeintake *fakeintake.Fakeintake) func(*Params) error {
	return func(p *Params) error {
		p.ResourceOptions = append(p.ResourceOptions, pulumi.DependsOn([]pulumi.Resource{fakeintake}))
		return withIntakeHostname(fakeintake.Scheme, fakeintake.Host, fakeintake.Port)(p)
	}
}

// WithLogs enables the log agent
func WithLogs() func(*Params) error {
	return func(p *Params) error {
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.String("logs_enabled: true"))
		return nil
	}
}

// WithAdditionalInstallParameters passes a list of parameters to the underlying installer
func WithAdditionalInstallParameters(parameters []string) func(*Params) error {
	return func(p *Params) error {
		p.AdditionalInstallParameters = parameters
		return nil
	}
}

// WithSkipAPIKeyInConfig does not add the API key in the Agent configuration file.
func WithSkipAPIKeyInConfig() func(*Params) error {
	return func(p *Params) error {
		p.SkipAPIKeyInConfig = true
		return nil
	}
}

// WithTags add tags to the agent configuration
func WithTags(tags []string) func(*Params) error {
	return func(p *Params) error {
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.Sprintf("tags: [%s]", strings.Join(tags, ", ")))
		return nil
	}
}

// WithHostname add hostname to the agent configuration
func WithHostname(hostname string) func(*Params) error {
	return func(p *Params) error {
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.Sprintf("hostname: %s", hostname))
		return nil
	}
}
