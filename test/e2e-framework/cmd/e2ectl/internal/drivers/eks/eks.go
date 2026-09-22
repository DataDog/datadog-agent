// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eks drives the Pulumi EKS scenario (Linux and optional Windows
// managed nodes, no Fargate). Its data-only config and topology validation are
// shared with the executor; Agent installation stays a separate operation
// with the non-Pulumi Helm installer against the exported kubeconfig.
package eks

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/pulumiworker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	eksconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// Driver is the eks driver: the shared Pulumi-executor lifecycle plus the
// cluster kubeconfig export, the same artifact the kind driver produces.
type Driver struct {
	pulumiworker.Lifecycle
}

// New returns the registered eks driver. The Helm installer carries no local
// image delivery hook: remote clusters install released chart versions only
// (the derivation rejects every other agent source for this base).
func New() *Driver {
	return &Driver{Lifecycle: *pulumiworker.New(
		workerclient.BaseEKS,
		"EKS cluster",
		"AWS EKS cluster (AL2023 Linux and optional Windows nodes, no Fargate) and optional ECS Fargate fakeintake (Pulumi)",
		[]installer.Installer{&installer.Kubernetes{}},
		exportKubeconfig,
	)}
}

func (d *Driver) Start(_ eksconfig.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	return d.Lifecycle.Start(cfg, entry, store)
}

func (d *Driver) Stop(_ eksconfig.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	return d.Lifecycle.Stop(cfg, entry, store)
}

// exportKubeconfig writes the cluster's kubeconfig next to the entry — the
// same single access path kubectl, the workload deployer and test attachment
// use on kind. The endpoint is private: the runner-profile VPN must be up for
// it to be reachable.
func exportKubeconfig(entry envstore.Entry, _ *envstore.Meta) error {
	var cluster outputs.ClusterOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "kubernetesCluster", &cluster); err != nil {
		return fmt.Errorf("reading Pulumi-provisioned cluster: %w", err)
	}
	if cluster.KubeConfig == "" {
		return fmt.Errorf("snapshot %s has no kubeconfig for the Pulumi-provisioned cluster", entry.SnapshotPath())
	}
	if err := os.WriteFile(entry.KubeconfigPath(), []byte(cluster.KubeConfig), 0o600); err != nil {
		return fmt.Errorf("writing the cluster kubeconfig: %w", err)
	}
	return nil
}
