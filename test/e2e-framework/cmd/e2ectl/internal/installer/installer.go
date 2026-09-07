// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

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
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	scriptinstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/installscript"
	helminstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/kubernetes/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

// Installer owns one agent installation method. The interfaces live here
// (not in the driver package) so drivers implement them structurally without
// importing the driver registry — no import cycle.
type Installer interface {
	// ID is the agent.install value.
	ID() string
	// Validate checks the installer's own agent-section rules.
	Validate(cfg *config.File) []error
	// Install installs (or upgrades) the agent on the environment.
	Install(cfg *config.File, entry envstore.Entry) error
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

// Validate implements driver.Installer: the helm chart's own rules.
func (k *Kubernetes) Validate(cfg *config.File) []error {
	var errs []error
	a := &cfg.Agent
	if a.Version == "" && a.Image == "" {
		errs = append(errs, fmt.Errorf("agent: either version or image is required when install is %q", k.ID()))
	}
	return errs
}

// Install implements driver.Installer.
func (k *Kubernetes) Install(cfg *config.File, entry envstore.Entry) error {
	if k.DeliverImage != nil && cfg.Agent.Image != "" {
		if err := k.DeliverImage(entry, cfg.Agent.Image); err != nil {
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
	if cfg.Agent.Image != "" {
		// In the upstream Datadog chart, agents.image.repository is the FULL
		// image path including the registry (the chart's image-path helper
		// renders repository:tag verbatim when repository is set).
		repository, tag := splitImageRef(cfg.Agent.Image)
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
		params.AgentVersion = cfg.Agent.Version
		params.ClusterAgentVersion = cfg.Agent.Version
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

// Validate implements driver.Installer: the install script's own rules.
func (h *HostScript) Validate(cfg *config.File) []error {
	var errs []error
	a := &cfg.Agent
	if a.Version == "" {
		errs = append(errs, fmt.Errorf("agent.version: required when install is %q", h.ID()))
	} else if !releasedVersionRegexp.MatchString(a.Version) {
		errs = append(errs, fmt.Errorf("agent.version: %q is not a released agent version (expected e.g. \"7.69.0\")", a.Version))
	}
	if a.Image != "" {
		errs = append(errs, fmt.Errorf("agent.image: not supported when install is %q", h.ID()))
	}
	return errs
}

// Install implements driver.Installer.
func (h *HostScript) Install(cfg *config.File, entry envstore.Entry) error {
	env, err := attach[environments.Host](entry)
	if err != nil {
		return err
	}
	if err := scriptinstaller.Install(nil, env, scriptinstaller.Params{
		AgentVersion: cfg.Agent.Version,
		AgentConfig:  cfg.Agent.Config,
		Integrations: cfg.Agent.Integrations,
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

var releasedVersionRegexp = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

var (
	_ Installer = (*Kubernetes)(nil)
	_ Updatable = (*Kubernetes)(nil)
	_ Installer = (*HostScript)(nil)
)
