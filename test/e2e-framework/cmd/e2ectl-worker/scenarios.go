// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	dockerconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/dockerhost"
	ec2config "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/ec2host"
	eksconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/fixtures"
	commonconfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	e2eostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	sceneks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	eksprovisioner "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/infraconfig"
)

// Explicit scenario registration. Only this binary imports Pulumi run functions.
// Each builder strict-decodes its shared typed config (the same schema, defaults
// and semantic rules the CLI used) and wraps one existing framework provisioner
// with fromTyped — never a second provisioning program.
var scenarios = map[string]Builder{
	workerclient.BaseEC2Host:    buildEC2Host,
	workerclient.BaseEKS:        buildEKS,
	workerclient.BaseDockerHost: buildDockerHost,
}

func buildEC2Host(params string, fixtureConfig fixtures.Config) (Executor, error) {
	// Same type, annotations, defaults and optional semantic validator as the CLI.
	p, err := ec2config.Schema.DecodeResolved([]byte(params), "environment.ec2-host")
	if err != nil {
		return Executor{}, err
	}
	desc, err := osDescriptor(p.OS)
	if err != nil {
		return Executor{}, err
	}
	opts := []ec2.Option{
		ec2.WithoutAgent(),
		ec2.WithEC2InstanceOptions(ec2.WithOSArch(desc, e2eostypes.ArchitectureFromString(p.Arch))),
	}
	if p.InstanceType != "" {
		opts = append(opts, ec2.WithEC2InstanceOptions(ec2.WithInstanceType(p.InstanceType)))
	}
	if !fixtureConfig.FakeIntake {
		opts = append(opts, ec2.WithoutFakeIntake())
	}
	return fromTyped[environments.Host](awshost.Provisioner(awshost.WithRunOptions(opts...))), nil
}

func buildEKS(params string, fixtureConfig fixtures.Config) (Executor, error) {
	p, err := eksconfig.Schema.DecodeResolved([]byte(params), "environment.eks")
	if err != nil {
		return Executor{}, err
	}
	clusterOpts := []sceneks.Option{sceneks.WithoutFargate()}
	if p.Linux {
		clusterOpts = append(clusterOpts, sceneks.WithLinuxNodeGroup())
	}
	if p.Windows {
		clusterOpts = append(clusterOpts, sceneks.WithWindowsNodeGroup())
	}
	opts := []sceneks.RunOption{
		sceneks.WithoutAgent(),
		sceneks.WithEKSOptions(clusterOpts...),
	}
	if !fixtureConfig.FakeIntake {
		opts = append(opts, sceneks.WithoutFakeIntake())
	}
	provOpts := []eksprovisioner.ProvisionerOption{
		eksprovisioner.WithRunOptions(opts...),
		eksprovisioner.WithExtraConfigParams(kubernetesVersion(p.Version)),
	}
	return fromTyped[environments.Kubernetes](eksprovisioner.Provisioner(provOpts...)), nil
}

// kubernetesVersion is the resolved stack config the AWS environment reads
// (ddinfra:kubernetesVersion) — the single version knob for the control plane
// and every enabled node group's AMI release.
func kubernetesVersion(version string) infraconfig.ConfigMap {
	cm := infraconfig.ConfigMap{}
	cm.Set(commonconfig.DDInfraConfigNamespace+":"+commonconfig.DDInfraKubernetesVersion, version, false)
	return cm
}

func buildDockerHost(params string, fixtureConfig fixtures.Config) (Executor, error) {
	p, err := dockerconfig.Schema.DecodeResolved([]byte(params), "environment.docker-host")
	if err != nil {
		return Executor{}, err
	}
	desc, err := dockerOSDescriptor(p.OS)
	if err != nil {
		return Executor{}, err
	}
	opts := []ec2docker.Option{
		ec2docker.WithoutAgent(),
		ec2docker.WithEC2VMOptions(ec2.WithOSArch(desc, e2eostypes.ArchitectureFromString(p.Arch))),
	}
	if p.InstanceType != "" {
		opts = append(opts, ec2docker.WithEC2VMOptions(ec2.WithInstanceType(p.InstanceType)))
	}
	if !fixtureConfig.FakeIntake {
		opts = append(opts, ec2docker.WithoutFakeIntake())
	}
	return fromTyped[environments.DockerHost](awsdocker.Provisioner(awsdocker.WithRunOptions(opts...))), nil
}

func osDescriptor(name string) (e2eostypes.Descriptor, error) {
	switch name {
	case "ubuntu-24.04":
		return e2eostypes.Ubuntu2404, nil
	case "ubuntu-22.04":
		return e2eostypes.Ubuntu2204, nil
	default:
		return e2eostypes.Descriptor{}, fmt.Errorf("no EC2 OS mapping for %q", name)
	}
}

// dockerOSDescriptor maps the typed OS names onto the e2e AMIs the ec2docker
// scenario is built on; the architecture is applied by WithOSArch.
func dockerOSDescriptor(name string) (e2eostypes.Descriptor, error) {
	switch name {
	case "ubuntu-24.04-e2e":
		return e2eostypes.Ubuntu2404E2E, nil
	case "ubuntu-22.04-e2e":
		return e2eostypes.Ubuntu2204E2E, nil
	default:
		return e2eostypes.Descriptor{}, fmt.Errorf("no Docker host OS mapping for %q", name)
	}
}
