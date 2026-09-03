// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// e2ectl-worker executes the Pulumi-linked or heavyweight jobs of e2ectl:
// cloud (EC2) provisioning and agent installation. The fast, local commands
// live in the core CLI (cmd/e2ectl) which stays Pulumi-free; this worker is the
// only part that links the provisioning SDKs. It is driven with a JSON job
// description and exits non-zero on failure.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	scriptinstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/installscript"
	helminstaller "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/kubernetes/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// job is the JSON contract between the core CLI and this worker.
type job struct {
	Action string `json:"action"`
	// EnvDir is the envstore entry directory.
	EnvDir string `json:"env_dir"`

	// EC2 provisioning fields.
	StackName    string `json:"stack_name,omitempty"`
	OS           string `json:"os,omitempty"`
	Arch         string `json:"arch,omitempty"`
	InstanceType string `json:"instance_type,omitempty"`
	FakeIntake   bool   `json:"fakeintake,omitempty"`

	// Agent installation fields.
	Version      string            `json:"version,omitempty"`
	Image        string            `json:"image,omitempty"`
	AgentConfig  string            `json:"agent_config,omitempty"`
	Integrations map[string]string `json:"integrations,omitempty"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: e2ectl-worker <job.json>")
		os.Exit(2)
	}
	var j job
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fatal("reading job: %v", err)
	}
	if err := json.Unmarshal(data, &j); err != nil {
		fatal("parsing job: %v", err)
	}
	if err := run(j); err != nil {
		fatal("%v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "e2ectl-worker: "+format+"\n", args...)
	os.Exit(1)
}

func run(j job) error {
	switch j.Action {
	case "provision-ec2":
		return provisionEC2(j)
	case "destroy-ec2":
		return destroyEC2(j)
	case "install-kind", "update-kind":
		return installOrUpdateKind(j)
	case "install-host":
		return installHost(j)
	default:
		return fmt.Errorf("unknown action %q", j.Action)
	}
}

func provisionEC2(j job) error {
	opts := []ec2.Option{
		ec2.WithoutAgent(), // the agent is installed separately, via `e2ectl install`
		ec2.WithEC2InstanceOptions(
			ec2.WithOSArch(osDescriptor(j.OS), e2eos.ArchitectureFromString(j.Arch)),
		),
	}
	if j.InstanceType != "" {
		opts = append(opts, ec2.WithEC2InstanceOptions(ec2.WithInstanceType(j.InstanceType)))
	}
	if !j.FakeIntake {
		opts = append(opts, ec2.WithoutFakeIntake())
	}

	ctx := standalone.NewContext(j.EnvDir)
	env, resources, err := standalone.ProvisionE[environments.Host](ctx, j.StackName,
		awshost.Provisioner(awshost.WithRunOptions(opts...)))
	if err != nil {
		return err
	}

	if err := provisioner.WriteSnapshotFile(snapshotPath(j), resources, map[string]any{
		"source": "e2ectl-worker-ec2",
	}); err != nil {
		return fmt.Errorf("writing snapshot: %w", err)
	}

	// print the connection info: the CLI surfaces it to the user
	fmt.Printf("ssh: %s@%s:%d\n", env.RemoteHost.Username, env.RemoteHost.Address, env.RemoteHost.Port)
	return nil
}

func destroyEC2(j job) error {
	opts := []ec2.Option{
		ec2.WithoutAgent(),
		ec2.WithEC2InstanceOptions(
			ec2.WithOSArch(osDescriptor(j.OS), e2eos.ArchitectureFromString(j.Arch)),
		),
	}
	if j.InstanceType != "" {
		opts = append(opts, ec2.WithEC2InstanceOptions(ec2.WithInstanceType(j.InstanceType)))
	}
	if !j.FakeIntake {
		opts = append(opts, ec2.WithoutFakeIntake())
	}
	ctx := standalone.NewContext(j.EnvDir)
	return standalone.Destroy(ctx, j.StackName,
		awshost.Provisioner(awshost.WithRunOptions(opts...)))
}

// installOrUpdateKind attaches the Kubernetes environment from the snapshot and
// installs (or upgrades) the agent with the Helm chart. update-kind first
// loads the local image into the kind cluster.
func installOrUpdateKind(j job) error {
	if j.Action == "update-kind" {
		if err := loadKindImage(j); err != nil {
			return err
		}
	}

	env, err := attachKubernetesEnv(j)
	if err != nil {
		return err
	}

	values := map[string]interface{}{}
	params := helminstaller.Params{Values: values}
	params.Namespace = "datadog"
	if j.Image != "" {
		// In the upstream Datadog chart, agents.image.repository is the FULL image
		// path including the registry (the chart's image-path helper renders
		// repository:tag verbatim when repository is set) — see
		// qa-e2ectl-implementation-notes.md.
		repository, tag := splitImageRef(j.Image)
		values["agents"] = map[string]interface{}{
			"image": map[string]interface{}{
				"repository": repository,
				"tag":        tag,
			},
		}
		// With a locally-built agent image the cluster-agent keeps the public
		// chart defaults: a custom tag such as "e2ectl-dev" is not semver and
		// the chart's version comparisons cannot parse it.
		params.ClusterAgentVersion = "latest"
	} else {
		params.AgentVersion = j.Version
		params.ClusterAgentVersion = j.Version
	}

	if err := helminstaller.Install(nil, env, params); err != nil {
		return err
	}

	if env.Agent != nil {
		return writeAgentToSnapshot(j, "agent", env.Agent.KubernetesAgentOutput)
	}
	return nil
}

func installHost(j job) error {
	env, err := attachHostEnv(j)
	if err != nil {
		return err
	}
	if err := scriptinstaller.Install(nil, env, scriptinstaller.Params{
		AgentVersion: j.Version,
		AgentConfig:  j.AgentConfig,
		Integrations: j.Integrations,
	}); err != nil {
		return err
	}
	if env.Agent != nil {
		return writeAgentToSnapshot(j, "agent", env.Agent.HostAgentOutput)
	}
	return nil
}

// attachKubernetesEnv rehydrates environments.Kubernetes from the snapshot
// without any provisioning.
func attachKubernetesEnv(j job) (*environments.Kubernetes, error) {
	p := provisioners.NewStaticStackProvisioner[environments.Kubernetes]("", snapshotPath(j))
	ctx := standalone.NewContext(j.EnvDir)
	env, _, err := standalone.ProvisionE[environments.Kubernetes](ctx, "attach", p)
	if err != nil {
		return nil, fmt.Errorf("attaching to the environment from its snapshot: %w", err)
	}
	return env, nil
}

func attachHostEnv(j job) (*environments.Host, error) {
	p := provisioners.NewStaticStackProvisioner[environments.Host]("", snapshotPath(j))
	ctx := standalone.NewContext(j.EnvDir)
	env, _, err := standalone.ProvisionE[environments.Host](ctx, "attach", p)
	if err != nil {
		return nil, fmt.Errorf("attaching to the environment from its snapshot: %w", err)
	}
	return env, nil
}

func loadKindImage(j job) error {
	// kind cluster name: from the snapshot's clusterName
	var cluster struct {
		ClusterName string `json:"clusterName"`
	}
	if err := provisioner.ReadSnapshotResource(snapshotPath(j), "kubernetesCluster", &cluster); err != nil {
		return err
	}
	cmd := exec.Command("kind", "load", "docker-image", j.Image, "--name", cluster.ClusterName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeAgentToSnapshot(j job, key string, output any) error {
	resources, meta, err := provisioner.ReadSnapshotFile(snapshotPath(j))
	if err != nil {
		return err
	}
	data, err := json.Marshal(output)
	if err != nil {
		return err
	}
	resources[key] = data
	return provisioner.WriteSnapshotFile(snapshotPath(j), resources, snapshotMeta(meta))
}

func snapshotPath(j job) string { return j.EnvDir + "/snapshot.json" }

func snapshotMeta(meta map[string]json.RawMessage) map[string]any {
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func osDescriptor(name string) e2eos.Descriptor {
	switch name {
	case "ubuntu-24.04":
		return e2eos.Ubuntu2404
	default:
		return e2eos.Ubuntu2204
	}
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
