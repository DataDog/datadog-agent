// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	AMD64Arch = "x86_64"
	ARM64Arch = "arm64"
)

// Latest 22.04 (jammy)
func LatestUbuntuAMI(e aws.Environment, arch string) (string, error) {
	return SearchAMI(e, "099720109477", "ubuntu/images/hvm-ssd/ubuntu-jammy-*", arch)
}

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

func GetAMIFromSSM(e aws.Environment, paramName string) (string, error) {
	result, err := ssm.LookupParameter(e.Ctx(), &ssm.LookupParameterArgs{
		Name: paramName,
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return "", err
	}
	return result.Value, nil
}
