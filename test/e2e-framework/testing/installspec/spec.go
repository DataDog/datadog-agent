// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installspec provides JSON-serializable install specifications for
// use by the e2e-install CLI. It captures the non-Pulumi-dependent agent
// configuration that would otherwise be carried by agentparams.Option closures.
package installspec

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/dockeragent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
)

// Spec is a JSON-serializable install specification. It is the stand-alone
// representation of what an automated E2E test or QA run wants installed on
// a provisioned environment.
type Spec struct {
	// EnvType must match the Descriptor.EnvType.
	EnvType string `json:"env_type"`
	// Cloud is used only for Kubernetes envs to resolve the internal registry.
	Cloud string `json:"cloud,omitempty"`
	// UseOperator selects the DatadogAgent Operator install path instead of Helm.
	UseOperator bool `json:"use_operator,omitempty"`
	// Host holds configuration for host-agent installs.
	Host *HostSpec `json:"host,omitempty"`
	// Kubernetes holds configuration for Helm/Operator installs.
	Kubernetes *KubernetesSpec `json:"kubernetes,omitempty"`
	// Docker holds configuration for Docker container installs.
	Docker *DockerSpec `json:"docker,omitempty"`
}

// HostSpec captures non-Pulumi host agent configuration.
type HostSpec struct {
	// Version selects the agent package version.
	Version VersionSpec `json:"version,omitempty"`
	// AgentConfig is the content to write to datadog.yaml (merged on top of
	// fakeintake/API-key defaults set by the installer).
	AgentConfig string `json:"agent_config,omitempty"`
	// SystemProbeConfig is the content to write to system-probe.yaml.
	SystemProbeConfig string `json:"system_probe_config,omitempty"`
	// SecurityAgentConfig is the content to write to security-agent.yaml.
	SecurityAgentConfig string `json:"security_agent_config,omitempty"`
	// Integrations maps check name to conf.yaml content.
	Integrations map[string]string `json:"integrations,omitempty"`
	// ExtraConfig are raw YAML snippets merged into datadog.yaml.
	ExtraConfig []string `json:"extra_config,omitempty"`
}

// VersionSpec mirrors agentparams.PackageVersion without Pulumi dependencies.
type VersionSpec struct {
	Major      string `json:"major,omitempty"`
	Minor      string `json:"minor,omitempty"`
	PipelineID string `json:"pipeline_id,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Flavor     string `json:"flavor,omitempty"`
}

// KubernetesSpec captures Helm/Operator agent configuration.
type KubernetesSpec struct {
	// HelmValues are raw YAML Helm value overrides (merged in order).
	HelmValues []string `json:"helm_values,omitempty"`
	// Namespace is the Kubernetes namespace for the agent release.
	Namespace string `json:"namespace,omitempty"`
	// DeployWindows enables Windows node agent deployment.
	DeployWindows bool `json:"deploy_windows,omitempty"`
}

// DockerSpec captures docker agent configuration.
type DockerSpec struct {
	// FullImagePath overrides image resolution when set.
	FullImagePath string `json:"full_image_path,omitempty"`
	// ImageTag overrides the resolved tag.
	ImageTag string `json:"image_tag,omitempty"`
	// Repository sets the docker image repository.
	Repository string `json:"repository,omitempty"`
	// FIPS requests the FIPS image variant.
	FIPS bool `json:"fips,omitempty"`
	// JMX requests the JMX image variant.
	JMX bool `json:"jmx,omitempty"`
	// EnvVars are additional environment variables injected into the agent container.
	EnvVars map[string]string `json:"env_vars,omitempty"`
}

// WriteToFile serializes s to a JSON file at path.
func WriteToFile(s *Spec, path string) error {
	data, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return fmt.Errorf("installspec.WriteToFile: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// ReadFromFile deserializes a Spec from a JSON file at path.
func ReadFromFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("installspec.ReadFromFile: %w", err)
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("installspec.ReadFromFile: %w", err)
	}
	return &s, nil
}

// FromHostAgentParams builds a HostSpec by evaluating agentparams.Option
// functions against an empty Params. Only non-Pulumi fields are captured.
func FromHostAgentParams(opts []agentparams.Option) *HostSpec {
	p := &agentparams.Params{
		Integrations: make(map[string]*agentparams.FileDefinition),
		Files:        make(map[string]*agentparams.FileDefinition),
	}
	for _, opt := range opts {
		_ = opt(p)
	}

	spec := &HostSpec{
		Version: VersionSpec{
			Major:      p.Version.Major,
			Minor:      p.Version.Minor,
			PipelineID: p.Version.PipelineID,
			Channel:    string(p.Version.Channel),
			Flavor:     p.Version.Flavor,
		},
		AgentConfig:         p.AgentConfig,
		SystemProbeConfig:   p.SystemProbeConfig,
		SecurityAgentConfig: p.SecurityAgentConfig,
		ExtraConfig:         p.ExtraAgentConfigRaw,
	}

	if len(p.Integrations) > 0 {
		spec.Integrations = make(map[string]string, len(p.Integrations))
		for name, fd := range p.Integrations {
			if fd != nil {
				spec.Integrations[name] = fd.Content
			}
		}
	}
	return spec
}

// HostAgentOptions converts a HostSpec to []agentparams.Option for use by
// hostagent.Install. Version defaults are NOT included here; the installer
// reads them from the runner profile as usual.
func (h *HostSpec) HostAgentOptions() []agentparams.Option {
	if h == nil {
		return nil
	}
	var opts []agentparams.Option
	if h.Version.Major != "" {
		opts = append(opts, agentparams.WithMajorVersion(h.Version.Major))
	}
	if h.Version.PipelineID != "" {
		opts = append(opts, agentparams.WithPipeline(h.Version.PipelineID))
	}
	if h.Version.Minor != "" {
		// Use WithVersion which handles channel+minor together ("7.x.y~beta-1" etc.)
		versionStr := h.Version.Major + "." + h.Version.Minor
		opts = append(opts, agentparams.WithVersion(versionStr))
	}
	if h.Version.Flavor != "" {
		opts = append(opts, agentparams.WithFlavor(h.Version.Flavor))
	}
	if h.AgentConfig != "" {
		opts = append(opts, agentparams.WithAgentConfig(h.AgentConfig))
	}
	if h.SystemProbeConfig != "" {
		opts = append(opts, agentparams.WithSystemProbeConfig(h.SystemProbeConfig))
	}
	if h.SecurityAgentConfig != "" {
		opts = append(opts, agentparams.WithSecurityAgentConfig(h.SecurityAgentConfig))
	}
	for _, extra := range h.ExtraConfig {
		e := extra
		opts = append(opts, func(p *agentparams.Params) error {
			p.ExtraAgentConfigRaw = append(p.ExtraAgentConfigRaw, e)
			return nil
		})
	}
	for name, content := range h.Integrations {
		n, c := name, content
		opts = append(opts, agentparams.WithIntegration(n, c))
	}
	return opts
}

// KubernetesAgentOptions converts a KubernetesSpec to []kubernetesagentparams.Option.
func (k *KubernetesSpec) KubernetesAgentOptions() []kubernetesagentparams.Option {
	if k == nil {
		return nil
	}
	var opts []kubernetesagentparams.Option
	for _, v := range k.HelmValues {
		opts = append(opts, kubernetesagentparams.WithHelmValues(v))
	}
	if k.Namespace != "" {
		opts = append(opts, kubernetesagentparams.WithNamespace(k.Namespace))
	}
	if k.DeployWindows {
		opts = append(opts, kubernetesagentparams.WithDeployWindows())
	}
	return opts
}

// DockerAgentOptions converts a DockerSpec to []dockeragent.Option.
func (d *DockerSpec) DockerAgentOptions() []dockeragent.Option {
	if d == nil {
		return nil
	}
	var opts []dockeragent.Option
	if d.FullImagePath != "" {
		opts = append(opts, dockeragent.WithFullImagePath(d.FullImagePath))
	}
	if d.ImageTag != "" {
		opts = append(opts, dockeragent.WithImageTag(d.ImageTag))
	}
	if d.Repository != "" {
		opts = append(opts, dockeragent.WithRepository(d.Repository))
	}
	if d.FIPS {
		opts = append(opts, dockeragent.WithFIPS())
	}
	if d.JMX {
		opts = append(opts, dockeragent.WithJMX())
	}
	for k, v := range d.EnvVars {
		opts = append(opts, dockeragent.WithEnvVar(k, v))
	}
	return opts
}

// CloudFromString converts a cloud string from the CLI ("aws", "az", "gcp") to a runner.Cloud.
func CloudFromString(s string) runner.Cloud {
	switch s {
	case "az", "azure":
		return runner.CloudAzure
	case "gcp":
		return runner.CloudGCP
	default:
		return runner.CloudAWS
	}
}
