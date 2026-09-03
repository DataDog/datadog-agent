// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package kinddriver creates and destroys local kind-based environments
// without Pulumi: it drives the kind CLI directly, runs the fakeintake as a
// local docker container, and writes an environment snapshot compatible with
// provisioners.StaticStackProvisioner.
package kinddriver

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// fakeintakeImage is the fakeintake image used for local environments.
const fakeintakeImage = "public.ecr.aws/datadog/fakeintake:latest"

// DefaultRCSigningKeySeed mirrors the e2e-framework constant of the same name
// (components/datadog/fakeintake). It is duplicated here because that package
// is Pulumi-linked and the core CLI must stay Pulumi-free: the seed is a stable
// public constant shared by every fakeintake deployment.
const DefaultRCSigningKeySeed = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// Start creates the kind cluster and the local fakeintake for entry, and writes
// the snapshot and metadata.
func Start(cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	meta := entry.Meta
	meta.KindName = entry.Name

	if err := createCluster(entry, cfg); err != nil {
		return fmt.Errorf("creating kind cluster: %w", err)
	}

	ipStr := "127.0.0.1"
	if cfg.FakeIntakeEnabled() {
		port, err := freePort()
		if err != nil {
			return err
		}
		container := fakeintakeContainerName(entry.Name)
		if err := runFakeintake(container, port); err != nil {
			return fmt.Errorf("starting fakeintake: %w", err)
		}
		ip, err := outboundIP()
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

// Stop destroys the kind cluster, the local fakeintake container and the entry.
func Stop(entry envstore.Entry, store *envstore.Store) error {
	if entry.Meta.KindName != "" {
		cmd := exec.Command("kind", "delete", "cluster", "--name", entry.Meta.KindName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("deleting kind cluster: %w", err)
		}
	}
	if entry.Meta.FakeIntakePort != 0 {
		container := fakeintakeContainerName(entry.Name)
		_ = exec.Command("docker", "rm", "-f", container).Run()
	}
	return store.Delete(entry.Name)
}

func createCluster(entry envstore.Entry, cfg *config.File) error {
	args := []string{"create", "cluster", "--name", entry.Name, "--kubeconfig", entry.KubeconfigPath()}
	if cfg.Environment.Kubernetes != nil && cfg.Environment.Kubernetes.Version != "" {
		args = append(args, "--image", "kindest/node:v"+cfg.Environment.Kubernetes.Version)
	}
	if cfg.Environment.Kubernetes != nil && cfg.Environment.Kubernetes.Nodes > 0 {
		cfgPath := filepath.Join(entry.Dir, "kind-config.yaml")
		if err := writeKindConfig(cfgPath, cfg.Environment.Kubernetes.Nodes); err != nil {
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

func runFakeintake(container string, port int) error {
	// -p <port>:80 binds every interface: pods must reach the fakeintake through
	// the host IP, so binding loopback only would hide it from the cluster.
	cmd := exec.Command("docker", "run", "-d", "--name", container,
		"-p", fmt.Sprintf("%d:80", port),
		fakeintakeImage,
		"--rc-key-data="+DefaultRCSigningKeySeed,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fakeintakeContainerName(envName string) string { return envName + "-fakeintake" }

func freePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// outboundIP returns an IP that is routable from containers on this host
// (the "UDP dial" trick used by the e2e-framework local fakeintake component).
func outboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}
