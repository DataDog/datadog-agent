// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/buildprovider"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/localpackage"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type artifactState struct {
	PackageVerification *localpackage.Verification `json:"packageVerification,omitempty"`
	Phase               string                     `json:"phase"`
	Result              agentbuild.Result          `json:"result"`
}

func artifactRequest(entry envstore.Entry, target agentbuild.Target) buildprovider.Request {
	return buildprovider.Request{Target: target, OutputDir: filepath.Join(entry.Dir, "artifacts")}
}
func readArtifact(entry envstore.Entry) (agentbuild.Result, error) {
	_, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return agentbuild.Result{}, err
	}
	var state artifactState
	if err = json.Unmarshal(meta["_agent_artifact"], &state); err != nil {
		return state.Result, fmt.Errorf("verified installed artifact missing; explicit preparation required (skip-build never repins source)")
	}
	if state.Phase != "installed" {
		return state.Result, fmt.Errorf("last artifact activation was not successful")
	}
	return state.Result, state.Result.Validate(state.Result.Target)
}
func artifactPhase(entry envstore.Entry, r agentbuild.Result, phase string) error {
	return provisioner.UpdateSnapshotResources(entry.SnapshotPath(), nil, map[string]any{"_agent_artifact": artifactState{Phase: phase, Result: r}})
}
func publishArtifact(entry envstore.Entry, r agentbuild.Result, updates provisioner.RawResources, packageVerification ...*localpackage.Verification) error {
	// Export the final receipt separately so the same built artifact can be
	// consumed by an existing-artifact provider without CLI state access.
	dir := filepath.Join(entry.Dir, "artifacts", "receipts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := agentbuild.Write(filepath.Join(dir, r.Summary().ID+".json"), r); err != nil {
		return err
	}
	state := artifactState{Phase: "installed", Result: r}
	if len(packageVerification) > 0 {
		state.PackageVerification = packageVerification[0]
	}
	return provisioner.UpdateSnapshotResources(entry.SnapshotPath(), updates, map[string]any{"_agent_artifact": state})
}
func artifactContext() (context.Context, context.CancelFunc) {
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(signalCtx, 45*time.Minute)
	return ctx, func() { cancel(); stop() }
}
func localTarget() agentbuild.Target {
	return agentbuild.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}
}
func legacyBinarySelection() (*config.BuildSelection, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(map[string]string{"repository": root})
	return &config.BuildSelection{Provider: "invoke-binary", Section: raw}, nil
}
func existingImageSelection(reference string) *config.BuildSelection {
	raw, _ := json.Marshal(map[string]string{"reference": reference})
	return &config.BuildSelection{Provider: "existing-image", Section: raw}
}
func attachClusterForInstall(entry envstore.Entry) (*environments.Kubernetes, error) {
	var out outputs.ClusterOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "kubernetesCluster", &out); err != nil {
		return nil, err
	}
	cluster := &components.KubernetesCluster{ClusterOutput: out}
	if err := cluster.Init(standalone.NewContext(entry.Dir)); err != nil {
		return nil, err
	}
	env := &environments.Kubernetes{KubernetesCluster: cluster}
	var fi outputs.FakeintakeOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "fakeIntake", &fi); err == nil {
		env.FakeIntake = &components.FakeIntake{FakeintakeOutput: fi}
	}
	return env, nil
}
func clusterTarget(ctx context.Context, env *environments.Kubernetes) (agentbuild.Target, error) {
	nodes, err := env.KubernetesCluster.Client().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return agentbuild.Target{}, err
	}
	var target agentbuild.Target
	for _, node := range nodes.Items {
		t := agentbuild.Target{OS: node.Status.NodeInfo.OperatingSystem, Arch: node.Status.NodeInfo.Architecture}
		if err := t.Validate(); err != nil {
			return t, err
		}
		if target.OS != "" && target != t {
			return t, fmt.Errorf("mixed-target clusters are unsupported for local artifacts")
		}
		target = t
	}
	return target, target.Validate()
}
