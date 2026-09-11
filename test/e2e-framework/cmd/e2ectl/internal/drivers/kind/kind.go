// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kind is the local kind driver: kind CLI + local docker fakeintake,
// no Pulumi anywhere. The whole cluster lifecycle is core-side; the snapshot
// is the same artifact every other driver produces.
package kind

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	kindconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/kind"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// Driver is the kind driver.
type Driver struct{}

// ID implements driver.Driver.
func (d *Driver) ID() string { return "kind" }

// Description describes the environment without inspecting local infrastructure.
func (d *Driver) Description() string {
	return "Local kind cluster and optional Docker fakeintake"
}

// Installers implements driver.Driver: the shared helm installer, with kind
// image loading as the delivery hook.
func (d *Driver) Installers() []installer.Installer {
	return []installer.Installer{
		&installer.Kubernetes{DeliverImage: installer.LoadKindImage},
	}
}

// Start implements driver.Driver.
func (d *Driver) Start(params kindconfig.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	meta := entry.Meta

	if err := createCluster(entry, params); err != nil {
		return fmt.Errorf("creating kind cluster: %w", err)
	}

	ipStr := "127.0.0.1"
	if cfg.FakeIntakeEnabled() {
		port, err := localinfra.RunFakeintake(entry.Name + "-fakeintake")
		if err != nil {
			return err
		}
		ip, err := localinfra.OutboundIP()
		if err != nil {
			return fmt.Errorf("resolving the routable host IP for the fakeintake: %w", err)
		}
		ipStr = ip.String()
		meta.FakeIntakePort = port
		meta.FakeIntakeURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	}

	kubeconfig, err := os.ReadFile(entry.KubeconfigPath())
	if err != nil {
		return fmt.Errorf("reading the kubeconfig written by kind: %w", err)
	}

	clusterKey, err := json.Marshal(map[string]string{
		"clusterName": entry.Name,
		"kubeConfig":  string(kubeconfig),
	})
	if err != nil {
		return err
	}
	resources := provisioner.RawResources{"kubernetesCluster": clusterKey}

	if cfg.FakeIntakeEnabled() {
		fiKey, err := json.Marshal(map[string]any{
			"host":   ipStr,
			"scheme": "http",
			"port":   meta.FakeIntakePort,
			"url":    fmt.Sprintf("http://%s:%d", ipStr, meta.FakeIntakePort),
		})
		if err != nil {
			return err
		}
		resources["fakeIntake"] = fiKey
	}

	meta.Status = envstore.StatusReady
	if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), resources, map[string]any{
		"source": "e2ectl-kind",
	}); err != nil {
		return err
	}
	entry.Meta = meta
	return store.UpdateMeta(entry)
}

// Stop implements driver.Driver. The cluster name is derived from the snapshot
// (the single source of truth), so no driver bookkeeping lives in the meta.
func (d *Driver) Stop(_ kindconfig.Config, _ *config.File, entry envstore.Entry, store *envstore.Store) error {
	var cluster struct {
		ClusterName string `json:"clusterName"`
	}
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "kubernetesCluster", &cluster); err != nil {
		// Half-created environment: no snapshot was ever written, but a cluster
		// may still exist — creation is deterministic, so the cluster is named
		// after the environment. Best-effort delete (creation may have failed
		// before the cluster existed), then clean the local fakeintake and
		// always remove the entry: a failed start must be recoverable with
		// plain `stop`.
		if delErr := deleteKindCluster(entry.Name); delErr != nil {
			fmt.Fprintf(os.Stderr, "warning: a kind cluster named %q may remain (delete it manually if it exists): %v\n", entry.Name, delErr)
		}
		_ = localinfra.StopFakeintake(entry.Name + "-fakeintake")
		return store.Delete(entry.Name)
	}
	if cluster.ClusterName != "" {
		if err := deleteKindCluster(cluster.ClusterName); err != nil {
			return fmt.Errorf("deleting kind cluster: %w", err)
		}
	}
	_ = localinfra.StopFakeintake(entry.Name + "-fakeintake")
	return store.Delete(entry.Name)
}

// deleteKindCluster removes a kind cluster by its deterministic name.
func deleteKindCluster(name string) error {
	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func createCluster(entry envstore.Entry, params kindconfig.Config) error {
	args := []string{"create", "cluster", "--name", entry.Name, "--kubeconfig", entry.KubeconfigPath()}
	if params.Version != "" {
		args = append(args, "--image", "kindest/node:v"+params.Version)
	}
	if params.Nodes > 0 {
		cfgPath := filepath.Join(entry.Dir, "kind-config.yaml")
		if err := writeKindConfig(cfgPath, params.Nodes); err != nil {
			return err
		}
		args = append(args, "--config", cfgPath)
	}
	cmd := exec.Command("kind", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeKindConfig(path string, workers int) error {
	body := "kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n"
	for i := 0; i < workers; i++ {
		body += "- role: worker\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
