// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

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
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// Driver is the kind driver.
type Driver struct{}

// ID implements driver.Driver.
func (d *Driver) ID() string { return "kind" }

// Section is the driver-owned config section (`environment.kind`).
type Section struct {
	// Version is a full kindest/node tag version, e.g. "1.31.0".
	Version string `yaml:"version,omitempty"`
	// Nodes is the number of worker nodes in addition to the control plane.
	Nodes int `yaml:"nodes,omitempty"`
}

var versionRegexp = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Validate implements driver.Driver: the kind section's own rules.
func (d *Driver) Validate(cfg *config.File) []error {
	if cfg.Environment.Section == nil {
		return nil // the section is optional; kind defaults apply
	}
	var s Section
	if err := config.StrictDecode(cfg.Environment.Section, &s); err != nil {
		return []error{fmt.Errorf("environment.kind: %v", err)}
	}
	var errs []error
	if s.Version != "" && !versionRegexp.MatchString(s.Version) {
		errs = append(errs, fmt.Errorf(
			"environment.kind.version: %q is not a full version (expected e.g. \"1.31.0\", it maps to kindest/node:v1.31.0)", s.Version))
	}
	if s.Nodes < 0 {
		errs = append(errs, fmt.Errorf("environment.kind.nodes: must be >= 0"))
	}
	return errs
}

// Installers implements driver.Driver: the shared helm installer, with kind
// image loading as the delivery hook.
func (d *Driver) Installers() []installer.Installer {
	return []installer.Installer{
		&installer.Kubernetes{DeliverImage: installer.LoadKindImage},
	}
}

// Start implements driver.Driver.
func (d *Driver) Start(cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	meta := entry.Meta

	if err := createCluster(entry, cfg); err != nil {
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
func (d *Driver) Stop(entry envstore.Entry, store *envstore.Store) error {
	var cluster struct {
		ClusterName string `json:"clusterName"`
	}
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "kubernetesCluster", &cluster); err != nil {
		// half-created environment: nothing to delete beyond the entry
		_ = localinfra.StopFakeintake(entry.Name + "-fakeintake")
		return store.Delete(entry.Name)
	}
	if cluster.ClusterName != "" {
		cmd := exec.Command("kind", "delete", "cluster", "--name", cluster.ClusterName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("deleting kind cluster: %w", err)
		}
	}
	_ = localinfra.StopFakeintake(entry.Name + "-fakeintake")
	return store.Delete(entry.Name)
}

func createCluster(entry envstore.Entry, cfg *config.File) error {
	args := []string{"create", "cluster", "--name", entry.Name, "--kubeconfig", entry.KubeconfigPath()}
	if cfg.Environment.Section != nil {
		var s Section
		if err := config.StrictDecode(cfg.Environment.Section, &s); err != nil {
			return err
		}
		if s.Version != "" {
			args = append(args, "--image", "kindest/node:v"+s.Version)
		}
		if s.Nodes > 0 {
			cfgPath := filepath.Join(entry.Dir, "kind-config.yaml")
			if err := writeKindConfig(cfgPath, s.Nodes); err != nil {
				return err
			}
			args = append(args, "--config", cfgPath)
		}
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
