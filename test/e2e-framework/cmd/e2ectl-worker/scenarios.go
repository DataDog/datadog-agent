// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	ec2config "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/fixtures"
	e2eostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// Explicit scenario registration. Only this binary imports Pulumi run functions.
var scenarios = map[string]Builder{
	workerclient.BaseEC2Host: buildEC2Host,
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
