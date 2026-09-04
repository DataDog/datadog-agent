// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package main

import (
	"fmt"
	"strings"

	e2eostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"

	"go.yaml.in/yaml/v3"
)

func init() {
	registerScenario(workerclient.BaseEC2Host, buildEC2Host)
}

// ec2Params is the scenario side of the driver's config section: one schema,
// the section IS the run function's params (strict-decoded here).
type ec2Params struct {
	OS           string `yaml:"os"`
	Arch         string `yaml:"arch"`
	InstanceType string `yaml:"instance-type,omitempty"`
}

// buildEC2Host strict-decodes the params and wraps the existing framework
// run function (the awshost provisioner over the EC2 scenario).
//
// Infra-only contract (§12 of the extensibility plan): the executor hands
// back an EMPTY, connectable VM — WithoutAgent and WithoutFakeIntake always.
// The fakeintake and the agent are deployed by the core, from the snapshot,
// exactly like every other environment.
func buildEC2Host(params string) (Executor, error) {
	if params == "" {
		return Executor{}, fmt.Errorf("empty params (os and arch required)")
	}
	var p ec2Params
	dec := yaml.NewDecoder(strings.NewReader(params))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return Executor{}, err
	}

	opts := []ec2.Option{
		ec2.WithoutAgent(),
		ec2.WithoutFakeIntake(),
		ec2.WithEC2InstanceOptions(
			ec2.WithOSArch(osDescriptor(p.OS), e2eostypes.ArchitectureFromString(p.Arch)),
		),
	}
	if p.InstanceType != "" {
		opts = append(opts, ec2.WithEC2InstanceOptions(ec2.WithInstanceType(p.InstanceType)))
	}

	return fromTyped[environments.Host](awshost.Provisioner(awshost.WithRunOptions(opts...))), nil
}

func osDescriptor(name string) e2eostypes.Descriptor {
	switch name {
	case "ubuntu-24.04":
		return e2eostypes.Ubuntu2404
	default:
		return e2eostypes.Ubuntu2204
	}
}
