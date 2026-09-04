// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package installer installs and updates the agent on an existing environment,
// in-process. It rehydrates the typed environment from the snapshot and uses
// the framework's Pulumi-free installers — after the outputs seam, no worker
// process is needed for any non-Pulumi operation.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	scriptinstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/installscript"
	helminstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/kubernetes/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

// Kind installs (or upgrades) the agent on a kind environment with the Helm chart.
// With a locally-built image (cfg.Agent.Image), the image is loaded into the kind
// cluster first and used via chart value overrides.
func Kind(entry envstore.Entry, cfg *config.File) error {
	if cfg.Agent.Image != "" {
		if err := loadKindImage(entry, cfg.Agent.Image); err != nil {
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
		// In the upstream Datadog chart, agents.image.repository is the FULL image
		// path including the registry (the chart's image-path helper renders
		// repository:tag verbatim when repository is set).
		repository, tag := splitImageRef(cfg.Agent.Image)
		values["agents"] = map[string]interface{}{
			"image": map[string]interface{}{
				"repository": repository,
				"tag":        tag,
			},
		}
		// A custom agent tag such as "7.99.0-e2ectl" is semver but the
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

// Host installs the agent on an ec2-host environment with the official install script.
func Host(entry envstore.Entry, cfg *config.File) error {
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

// attach rehydrates a typed environment from the snapshot without any provisioning.
func attach[Env any](entry envstore.Entry) (*Env, error) {
	p := provisioner.NewStaticStackProvisioner[Env]("", entry.SnapshotPath())
	ctx := standalone.NewContext(entry.Dir)
	env, _, err := standalone.ProvisionE[Env](ctx, "attach", p)
	if err != nil {
		return nil, fmt.Errorf("attaching to the environment from its snapshot: %w", err)
	}
	return env, nil
}

// loadKindImage loads a locally-built docker image into the kind cluster.
func loadKindImage(entry envstore.Entry, image string) error {
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
	resources, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return err
	}
	data, err := json.Marshal(output)
	if err != nil {
		return err
	}
	resources["agent"] = data
	return provisioner.WriteSnapshotFile(entry.SnapshotPath(), resources, snapshotMeta(meta))
}

func snapshotMeta(meta map[string]json.RawMessage) map[string]any {
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
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
