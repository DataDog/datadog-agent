// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer holds the SHARED agent installers, referenced by the
// drivers, and the Installer/Updatable contracts the drivers advertise.
// Installers operate on typed environments rehydrated from the snapshot —
// they do not know which driver created the environment, so one instance
// serves every compatible base.
//
// NewKubernetes is parameterized by the image-delivery hook: how a locally
// built agent image reaches the cluster (kind load for kind, a registry push
// for remote clusters). That hook is the entire semantic difference between
// cluster drivers.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	helmconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/helm"
	scriptconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/script"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	scriptinstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/installscript"
	helminstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/kubernetes/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"go.yaml.in/yaml/v3"
)

// Installer owns one agent installation method. The interfaces live here
// (not in the driver package) so drivers implement them structurally without
// importing the driver registry — no import cycle. The agent section is
// installer-typed, exactly like the environment section is driver-typed:
// ID() is both the agent.install value and the agent section name, and the
// installer's own schema validates and example-generates that section.
type Installer interface {
	// ID is the agent.install value; it is also the agent section name.
	ID() string
	// AgentExample generates this installer's typed agent section for starter
	// configs, from its own schema.
	AgentExample() (*yaml.Node, error)
	// Artifact reports the version and image requested by the decoded agent
	// section, for bookkeeping (envstore meta). Zero values where absent.
	Artifact(cfg *config.File) (version, image string, err error)
	// Validate checks the installer's own agent-section rules: schema decode
	// plus its semantic hook.
	Validate(cfg *config.File) []error
	// Install installs (or upgrades) the agent on the environment.
	Install(cfg *config.File, entry envstore.Entry) error
}

// decodeAgentSection decodes the installer-owned agent section against its
// schema, preserving the original file positions in errors. The section is
// optional in the envelope; an absent section decodes as empty and the
// schema's required/default rules then apply.
func decodeAgentSection[A any](schema *configschema.Schema[A], cfg *config.File, id string) (A, error) {
	path := "agent." + id
	if cfg.Agent.SectionNode != nil {
		value, _, err := schema.DecodeNode(cfg.Agent.SectionNode, path)
		return value, err
	}
	value, _, err := schema.Decode(cfg.Agent.Section, path)
	return value, err
}

// Updatable is the optional local-iteration capability: every environment
// should end up updatable, but drivers can ship without it and grow it later.
type Updatable interface {
	Installer
	Update(cfg *config.File, entry envstore.Entry) error
}

// Kubernetes installs or upgrades the agent with the Helm chart on any
// Kubernetes environment (kind, and later remote clusters).
type Kubernetes struct {
	// DeliverImage makes a locally-built image available to the cluster
	// (e.g. kind load, or a registry push). May be nil when only released
	// versions are installed.
	DeliverImage func(entry envstore.Entry, image string) error
}

// ID implements driver.Installer.
func (k *Kubernetes) ID() string { return "helm" }

// AgentExample implements driver.Installer: the helm section schema's example.
func (k *Kubernetes) AgentExample() (*yaml.Node, error) { return helmconfig.Schema.Example(nil) }

// Artifact implements driver.Installer.
func (k *Kubernetes) Artifact(cfg *config.File) (string, string, error) {
	section, err := decodeAgentSection(helmconfig.Schema, cfg, k.ID())
	return section.Version, section.Image, err
}

// Validate implements driver.Installer: the helm section's own rules.
func (k *Kubernetes) Validate(cfg *config.File) []error {
	if _, err := decodeAgentSection(helmconfig.Schema, cfg, k.ID()); err != nil {
		return []error{err}
	}
	return nil
}

// Install implements driver.Installer.
func (k *Kubernetes) Install(cfg *config.File, entry envstore.Entry) error {
	section, err := decodeAgentSection(helmconfig.Schema, cfg, k.ID())
	if err != nil {
		return err
	}
	if k.DeliverImage != nil && section.Image != "" {
		if err := k.DeliverImage(entry, section.Image); err != nil {
			return err
		}
	}
	env, err := attach[environments.Kubernetes](entry)
	if err != nil {
		return err
	}

	values := map[string]interface{}{}
	params := helminstaller.Params{Values: values}
	params.Namespace = "datadog"
	if section.Image != "" {
		// In the upstream Datadog chart, agents.image.repository is the FULL
		// image path including the registry (the chart's image-path helper
		// renders repository:tag verbatim when repository is set).
		repository, tag := splitImageRef(section.Image)
		values["agents"] = map[string]interface{}{
			"image": map[string]interface{}{
				"repository": repository,
				"tag":        tag,
			},
		}
		// A custom agent tag such as "7.99.0-e2ectl" is semver, but the
		// cluster-agent keeps the public chart defaults.
		params.ClusterAgentVersion = "latest"
	} else {
		params.AgentVersion = section.Version
		params.ClusterAgentVersion = section.Version
	}

	if err := helminstaller.Install(nil, env, params); err != nil {
		return err
	}
	if env.Agent != nil {
		return writeAgentToSnapshot(entry, env.Agent.KubernetesAgentOutput)
	}
	return nil
}

// Update implements driver.Updatable: the same install path (the chart
// installer upgrades an existing release), with the image delivered first.
func (k *Kubernetes) Update(cfg *config.File, entry envstore.Entry) error {
	return k.Install(cfg, entry)
}

// HostScript installs the agent on a host environment (ec2-host, and later
// local host drivers) with the official install script.
type HostScript struct{}

// ID implements driver.Installer.
func (h *HostScript) ID() string { return "script" }

// AgentExample implements driver.Installer: the script section schema's example.
func (h *HostScript) AgentExample() (*yaml.Node, error) { return scriptconfig.Schema.Example(nil) }

// Artifact implements driver.Installer. Host installs have no image field —
// it does not exist in the script section.
func (h *HostScript) Artifact(cfg *config.File) (string, string, error) {
	section, err := decodeAgentSection(scriptconfig.Schema, cfg, h.ID())
	return section.Version, "", err
}

// Validate implements driver.Installer: the script section's own rules.
func (h *HostScript) Validate(cfg *config.File) []error {
	if _, err := decodeAgentSection(scriptconfig.Schema, cfg, h.ID()); err != nil {
		return []error{err}
	}
	return nil
}

// Install implements driver.Installer.
func (h *HostScript) Install(cfg *config.File, entry envstore.Entry) error {
	section, err := decodeAgentSection(scriptconfig.Schema, cfg, h.ID())
	if err != nil {
		return err
	}
	env, err := attach[environments.Host](entry)
	if err != nil {
		return err
	}
	if err := scriptinstaller.Install(nil, env, scriptinstaller.Params{
		AgentVersion: section.Version,
		AgentConfig:  section.Config,
		Integrations: section.Integrations,
	}); err != nil {
		return err
	}
	if env.Agent != nil {
		return writeAgentToSnapshot(entry, env.Agent.HostAgentOutput)
	}
	return nil
}

// attach rehydrates a typed environment from the snapshot without any
// provisioning — the executor is long gone by the time installers run.
func attach[Env any](entry envstore.Entry) (*Env, error) {
	p := provisioner.NewStaticStackProvisioner[Env]("", entry.SnapshotPath())
	ctx := standalone.NewContext(entry.Dir)
	env, _, err := standalone.ProvisionE[Env](ctx, "attach", p)
	if err != nil {
		return nil, fmt.Errorf("attaching to the environment from its snapshot: %w", err)
	}
	return env, nil
}

// LoadKindImage delivers a locally-built docker image into a kind cluster.
func LoadKindImage(entry envstore.Entry, image string) error {
	var cluster struct {
		ClusterName string `json:"clusterName"`
	}
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "kubernetesCluster", &cluster); err != nil {
		return err
	}
	cmd := exec.Command("kind", "load", "docker-image", image, "--name", cluster.ClusterName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// writeAgentToSnapshot persists the agent component into the snapshot so the
// snapshot stays the single source of truth.
func writeAgentToSnapshot(entry envstore.Entry, output any) error {
	data, err := json.Marshal(output)
	if err != nil {
		return err
	}
	return provisioner.UpdateSnapshotResource(entry.SnapshotPath(), "agent", data)
}

// splitImageRef splits "gcr.io/datadoghq/agent:tag" into
// ("gcr.io/datadoghq/agent", "tag").
func splitImageRef(ref string) (repository, tag string) {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == ':' {
			return ref[:i], ref[i+1:]
		}
	}
	return ref, ""
}

var (
	_ Installer = (*Kubernetes)(nil)
	_ Updatable = (*Kubernetes)(nil)
	_ Installer = (*HostScript)(nil)
)
