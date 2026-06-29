// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ec2 provides helpers for creating and managing AWS EC2 resources.
package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// AMI architecture constants.
const (
	AMD64Arch = "x86_64"
	ARM64Arch = "arm64"
)

// LatestUbuntuAMI returns the latest Ubuntu 22.04 (jammy) AMI for the given architecture.
func LatestUbuntuAMI(e aws.Environment, arch string) (string, error) {
	return SearchAMI(e, "099720109477", "ubuntu/images/hvm-ssd/ubuntu-jammy-*", arch)
}

// SearchAMI searches for an AMI by owner, name pattern, and architecture.
func SearchAMI(e aws.Environment, owner, name, arch string) (string, error) {
	image, err := ec2.LookupAmi(e.Ctx(), &ec2.LookupAmiArgs{
		MostRecent: pulumi.BoolRef(true),
		Filters: []ec2.GetAmiFilter{
			{
				Name: "name",
				Values: []string{
					name,
				},
			},
			{
				Name: "virtualization-type",
				Values: []string{
					"hvm",
				},
			},
			{
				Name: "architecture",
				Values: []string{
					arch,
				},
			},
		},
		Owners: []string{
			owner,
		},
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return "", err
	}

	if image == nil {
		return "", fmt.Errorf("unable to find AMI with owner: %s, name: %s, arch: %s", owner, name, arch)
	}

	return image.Id, nil
}

// GetAMIFromSSM retrieves an AMI ID from an SSM parameter.
func GetAMIFromSSM(e aws.Environment, paramName string) (string, error) {
	result, err := ssm.LookupParameter(e.Ctx(), &ssm.LookupParameterArgs{
		Name: paramName,
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return "", err
	}
	return result.Value, nil
}
