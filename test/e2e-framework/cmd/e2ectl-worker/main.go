// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// e2ectl-worker executes the Pulumi-linked jobs of e2ectl: cloud (EC2)
// provisioning. Everything that does not require Pulumi — kind clusters, local
// fakeintake, agent installation and updates via the framework's Pulumi-free
// installers — runs directly in the core CLI. This worker is driven with a
// JSON job description (cmd/e2ectl/workerclient.Job) and exits non-zero on
// failure.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	e2eostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: e2ectl-worker <job.json>")
		os.Exit(2)
	}
	var j workerclient.Job
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

func run(j workerclient.Job) error {
	switch j.Action {
	case "provision-ec2":
		return provisionEC2(j)
	case "destroy-ec2":
		return standalone.Destroy(standalone.NewContext(j.EnvDir), j.StackName, ec2Provisioner(j))
	default:
		return fmt.Errorf("unknown action %q", j.Action)
	}
}

func provisionEC2(j workerclient.Job) error {
	ctx := standalone.NewContext(j.EnvDir)
	env, resources, err := standalone.ProvisionE[environments.Host](ctx, j.StackName, ec2Provisioner(j))
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

// ec2Provisioner builds the awshost provisioner from the job: a bare VM
// (the agent is installed separately, via `e2ectl install`), with an optional
// cloud fakeintake.
func ec2Provisioner(j workerclient.Job) provisioner.TypedProvisioner[environments.Host] {
	opts := []ec2.Option{
		ec2.WithoutAgent(),
		ec2.WithEC2InstanceOptions(
			ec2.WithOSArch(osDescriptor(j.OS), e2eostypes.ArchitectureFromString(j.Arch)),
		),
	}
	if j.InstanceType != "" {
		opts = append(opts, ec2.WithEC2InstanceOptions(ec2.WithInstanceType(j.InstanceType)))
	}
	if !j.FakeIntake {
		opts = append(opts, ec2.WithoutFakeIntake())
	}
	return awshost.Provisioner(awshost.WithRunOptions(opts...))
}

func snapshotPath(j workerclient.Job) string { return j.EnvDir + "/snapshot.json" }

func osDescriptor(name string) e2eostypes.Descriptor {
	switch name {
	case "ubuntu-24.04":
		return e2eostypes.Ubuntu2404
	default:
		return e2eostypes.Ubuntu2204
	}
}
