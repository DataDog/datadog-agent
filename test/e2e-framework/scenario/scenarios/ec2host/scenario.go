// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2host

import (
	"context"
	"fmt"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// osDescriptor maps the schema enum value to the framework OS descriptor.
func osDescriptor(name string) (e2eos.Descriptor, error) {
	switch name {
	case "ubuntu-22.04":
		return e2eos.Ubuntu2204, nil
	case "debian-12":
		return e2eos.Debian12, nil
	case "amazon-linux-2023":
		return e2eos.AmazonLinux2023, nil
	default:
		return e2eos.Descriptor{}, fmt.Errorf("unknown os %q", name)
	}
}

// archFromString maps the schema arch enum values ("x86_64", "arm64") to the
// framework Architecture type. It returns an explicit error rather than panicking
// so callers get a clear message for invalid inputs.
func archFromString(arch string) (e2eos.Architecture, error) {
	switch arch {
	case "x86_64", "":
		return e2eos.AMD64Arch, nil
	case "arm64":
		return e2eos.ARM64Arch, nil
	default:
		return "", fmt.Errorf("unknown arch %q (valid: x86_64, arm64)", arch)
	}
}

// Provisioner adapts canonical EC2HostParams to the existing awshost provisioner.
func Provisioner(p *EC2HostParams) (provisioners.TypedProvisioner[environments.Host], error) {
	osDesc, err := osDescriptor(p.OS)
	if err != nil {
		return nil, err
	}

	arch, err := archFromString(p.Arch)
	if err != nil {
		return nil, err
	}

	agentOpts, err := p.Agent.ToOptions()
	if err != nil {
		return nil, err
	}

	fakeOpts, err := p.Fakeintake.ToOptions()
	if err != nil {
		return nil, err
	}

	// Set OS with explicit architecture; append any caller-supplied VM tweaks.
	runOpts := []ec2.Option{
		ec2.WithEC2InstanceOptions(
			append([]ec2.VMOption{ec2.WithOSArch(osDesc, arch)}, p.InstanceOptions...)...,
		),
	}

	if p.Agent.Install {
		runOpts = append(runOpts, ec2.WithAgentOptions(agentOpts...))
	} else {
		runOpts = append(runOpts, ec2.WithoutAgent())
	}

	if p.Fakeintake.Enabled {
		runOpts = append(runOpts, ec2.WithFakeIntakeOptions(fakeOpts...))
	} else {
		runOpts = append(runOpts, ec2.WithoutFakeIntake())
	}

	return awshost.Provisioner(awshost.WithRunOptions(runOpts...)), nil
}

// RunCommandParams are the parameters for the run-command action.
type RunCommandParams struct {
	Command string `scenario:"name=command,required,help=Shell command to run over SSH"`
}

// Scenario returns the unified ec2-host scenario definition.
func Scenario() scenario.Scenario[environments.Host] {
	return scenario.Scenario[environments.Host]{
		Name:        "ec2-host",
		Description: "AWS EC2 VM with the Datadog Agent",
		NewParams:   func() any { return NewParams() },
		Provisioner: func(p any) (provisioners.TypedProvisioner[environments.Host], error) {
			return Provisioner(p.(*EC2HostParams))
		},
		Actions: map[string]scenario.Action[environments.Host]{
			"restart-agent": {
				Description: "Restart the Datadog Agent",
				Run: func(_ context.Context, env *environments.Host, _ any) error {
					return env.Agent.Client.Restart()
				},
			},
			"run-command": {
				Description: "Run a shell command on the host over SSH",
				NewParams:   func() any { return &RunCommandParams{} },
				Run: func(_ context.Context, env *environments.Host, p any) error {
					_, err := env.RemoteHost.Execute(p.(*RunCommandParams).Command)
					return err
				},
			},
		},
	}
}

// Register registers the ec2-host scenario in the package registry.
func Register() { scenario.Register(Scenario()) }
