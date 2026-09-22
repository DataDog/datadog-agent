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
// Kubernetes is parameterized by a local-image delivery hook. Current artifact
// providers require the verified image preloaded on each node (kind load for
// kind); registry acquisition/push is not implemented by this adapter.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/buildprovider"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	helmconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/helm"
	scriptconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/script"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	scriptinstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/installscript"
	helminstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/kubernetes/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
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

// decodeAgentSection decodes the DERIVED installer-owned agent section against
// its schema. The section is produced by the derivation, never parsed from
// user YAML, so there is no original node to preserve positions from.
func decodeAgentSection[A any](schema *configschema.Schema[A], cfg *config.File, id string) (A, error) {
	value, _, err := schema.Decode(cfg.Agent.Section, "agent."+id)
	return value, err
}

// Updatable is the optional local-iteration capability: every environment
// should end up updatable, but drivers can ship without it and grow it later.
// Update owns its artifact preparation: each installer decides what "prepare"
// means (rebuild a binary, build a dev image, or nothing under skipBuild) —
// the CLI never dispatches on installer-specific artifact knowledge.
type Updatable interface {
	Installer
	Update(cfg *config.File, entry envstore.Entry, skipBuild bool) error
}

// Kubernetes installs or upgrades the agent with the Helm chart on any
// Kubernetes environment (kind, and later remote clusters).
type Kubernetes struct {
	// DeliverImage preloads the exact verified local image/tag on cluster nodes
	// (e.g. kind load). May be nil when only released versions are installed.
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
	section, err := decodeAgentSection(helmconfig.Schema, cfg, k.ID())
	if err != nil {
		return []error{err}
	}
	if cfg.Agent.Build == nil && section.Version == "" && section.Image == "" {
		return []error{fmt.Errorf("agent.helm: either version or image is required without agent.build")}
	}
	if err := ValidateReceiver(k, cfg); err != nil {
		return []error{err}
	}
	if cfg.Agent.Build != nil {
		if err := buildprovider.Images.Validate(cfg.Agent.Build); err != nil {
			return []error{err}
		}
		if section.Image != "" {
			return []error{fmt.Errorf("agent.helm.image and agent.build are mutually exclusive; the provider owns the reference")}
		}
	}
	if cfg.Agent.Receiver != nil {
		var values map[string]interface{}
		if err := yaml.Unmarshal([]byte(section.Values), &values); err != nil {
			return []error{fmt.Errorf("invalid Helm values")}
		}
		if err := helminstaller.ValidateRoutingValues(values); err != nil {
			return []error{err}
		}
	}
	return nil
}

// Install implements driver.Installer.
func (k *Kubernetes) Install(cfg *config.File, entry envstore.Entry) error {
	return k.install(cfg, entry, false)
}

func (k *Kubernetes) install(cfg *config.File, entry envstore.Entry, skip bool) error {
	section, err := decodeAgentSection(helmconfig.Schema, cfg, k.ID())
	if err != nil {
		return err
	}
	if errs := k.Validate(cfg); len(errs) > 0 {
		return config.NewErrors(errs)
	}
	routing, err := k.PrepareRouting(cfg, entry)
	if err != nil {
		return err
	}
	apiKey := ""
	if routing != nil {
		apiKey, err = bindAPIKey(routing)
		if err != nil {
			return err
		}
	}

	env, err := attachClusterForInstall(entry)
	if err != nil {
		return err
	}
	ctx, cancel := artifactContext()
	defer cancel()
	var artifact *agentbuild.Result
	selection := cfg.Agent.Build
	if selection == nil && section.Image != "" {
		selection = existingImageSelection(section.Image)
	}
	if selection != nil {
		if k.DeliverImage == nil {
			return fmt.Errorf("environment cannot deliver local image artifacts")
		}
		target, err := clusterTarget(ctx, env)
		if err != nil {
			return err
		}
		var result agentbuild.Result
		if skip {
			result, err = readArtifact(entry)
			if err == nil {
				err = result.Validate(target)
			}
			if err == nil {
				err = (agentbuild.Adapter{}).VerifyImage(ctx, result)
			}
		} else {
			if routing != nil {
				if err := buildprovider.Images.ValidateRouting(selection); err != nil {
					return err
				}
			}
			prepared, prepErr := buildprovider.Images.Prepare(ctx, selection, artifactRequest(entry, target))
			result, err = prepared.Result, prepErr
			if err == nil && routing != nil && result.Profile == nil {
				return fmt.Errorf("explicit receiver requires a compatible provider receipt; arbitrary existing images are not attested")
			}
			if err == nil {
				result, err = (agentbuild.Adapter{}).DeliverableImage(ctx, result)
			}
		}
		if err != nil {
			return err
		}
		if routing != nil && result.Profile == nil {
			return fmt.Errorf("explicit receiver requires producer capability receipt")
		}
		if err := k.DeliverImage(entry, result.Image.Delivered); err != nil {
			return err
		}
		artifact = &result
	}

	values := map[string]interface{}{}
	if strings.TrimSpace(section.Values) != "" {
		rendered := section.Values
		if env.FakeIntake != nil {
			fi := env.FakeIntake.FakeintakeOutput
			if fi.AgentURL == "" {
				fi.AgentURL = fi.URL
			} // legacy kind snapshots already used a producer-routable URL
			rendered = strings.NewReplacer(
				"{{FAKEINTAKE_URL}}", fi.AgentURL,
				"{{FAKEINTAKE_HOST}}", fi.Host,
				"{{FAKEINTAKE_PORT}}", fmt.Sprintf("%d", fi.Port),
			).Replace(rendered)
		}
		var extra map[string]interface{}
		if err := yaml.Unmarshal([]byte(rendered), &extra); err != nil {
			return fmt.Errorf("agent.helm.values: %w", err)
		}
		mergeValues(values, extra)
	}
	// The framework's scenarios tag the agent with the Pulumi stack id
	// (stackid:<stack>); the e2ectl analog is the environment name. Suites
	// like the containers k8sSuite assert it on cluster-scoped metrics.
	setAgentTag(values, "stackid:"+entry.Name)
	params := helminstaller.Params{Values: values, Routing: routing, APIKey: apiKey}
	params.Namespace = "datadog"
	if artifact != nil {
		repository, tag := splitImageRef(artifact.Image.Delivered)
		params.Image = &helminstaller.ImageArtifact{Repository: repository, Tag: tag, LocalImageID: artifact.Image.ID}
		params.Profile = artifact.Profile
		params.AgentVersion = tag
		// The core image is not a rebuilt Cluster Agent. Keep its separately tested
		// released image instead of silently retagging DCA or using mutable latest.
		params.ClusterAgentVersion = "7.83.0"
	} else {
		params.AgentVersion = section.Version
		params.ClusterAgentVersion = section.Version
	}
	if artifact != nil {
		if err := artifactPhase(entry, *artifact, "activating"); err != nil {
			return err
		}
	}

	if err := helminstaller.Install(ctx, env, params); err != nil {
		if artifact != nil {
			_ = artifactPhase(entry, *artifact, "failed")
		}
		return err
	}
	if env.Agent != nil {
		if artifact != nil {
			data, err := json.Marshal(env.Agent.KubernetesAgentOutput)
			if err != nil {
				return err
			}
			return publishArtifact(entry, *artifact, provisioner.RawResources{"agent": data})
		}
		return writeAgentToSnapshot(entry, env.Agent.KubernetesAgentOutput)
	}
	return nil
}

// Update uses the same provider/consumer path as Install. Existing-image sources
// never build; skipBuild verifies the installed receipt instead of the source.
func (k *Kubernetes) Update(cfg *config.File, entry envstore.Entry, skipBuild bool) error {
	return k.install(cfg, entry, skipBuild)
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
	if cfg.Agent.Build != nil {
		return []error{fmt.Errorf("script installer does not consume agent.build; use package")}
	}
	section, err := decodeAgentSection(scriptconfig.Schema, cfg, h.ID())
	if err != nil {
		return []error{err}
	}
	if err := ValidateReceiver(h, cfg); err != nil {
		return []error{err}
	}
	if cfg.Agent.Receiver != nil {
		if _, err := receivers.ValidateConfig(section.Config); err != nil {
			return []error{err}
		}
	}
	return nil
}

// Install implements driver.Installer.
func (h *HostScript) Install(cfg *config.File, entry envstore.Entry) error {
	section, err := decodeAgentSection(scriptconfig.Schema, cfg, h.ID())
	if err != nil {
		return err
	}
	if errs := h.Validate(cfg); len(errs) > 0 {
		return config.NewErrors(errs)
	}
	routing, err := h.PrepareRouting(cfg, entry)
	if err != nil {
		return err
	}
	apiKey := ""
	if routing != nil {
		apiKey, err = bindAPIKey(routing)
		if err != nil {
			return err
		}
	}

	env, err := attachHostForInstall(entry)
	if err != nil {
		return err
	}
	if err := scriptinstaller.Install(nil, env, scriptinstaller.Params{
		AgentVersion: section.Version,
		Routing:      routing, APIKey: apiKey,
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
	return provisioner.UpdateSnapshotResources(entry.SnapshotPath(), provisioner.RawResources{"agent": data}, map[string]any{"_agent_artifact": nil})
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

// mergeValues deep-merges src into dst (maps merge recursively, everything
// else is overwritten by src).
func mergeValues(dst, src map[string]interface{}) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := dst[k].(map[string]interface{}); ok {
				mergeValues(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

// setAgentTag appends tag to datadog.tags in the Helm values map without
// clobbering other chart values.
func setAgentTag(values map[string]interface{}, tag string) {
	datadog, ok := values["datadog"].(map[string]interface{})
	if !ok {
		datadog = map[string]interface{}{}
		values["datadog"] = datadog
	}
	var tags []string
	switch existing := datadog["tags"].(type) {
	case []string:
		tags = existing
	case []interface{}:
		for _, value := range existing {
			if tag, ok := value.(string); ok {
				tags = append(tags, tag)
			}
		}
	}
	datadog["tags"] = append(tags, tag)
}
