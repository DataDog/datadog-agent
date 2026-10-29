// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eks

import (
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	awsEc2 "github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NoInternetEgressRules allows the VPC's primary and secondary IPv4 networks and
// HTTPS to regional S3. ECR image layers use the account's S3 gateway endpoint,
// whose destinations are service IPs rather than addresses in the VPC's CIDRs.
func NoInternetEgressRules(e aws.Environment) (awsEc2.SecurityGroupEgressArray, error) {
	vpc, err := awsEc2.LookupVpc(e.Ctx(), &awsEc2.LookupVpcArgs{Id: pulumi.StringRef(e.DefaultVPCID())}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return nil, fmt.Errorf("failed to look up VPC %s: %w", e.DefaultVPCID(), err)
	}
	s3, err := awsEc2.GetPrefixList(e.Ctx(), &awsEc2.GetPrefixListArgs{
		Name: pulumi.StringRef("com.amazonaws." + e.Region() + ".s3"),
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return nil, fmt.Errorf("failed to look up regional S3 prefix list: %w", err)
	}
	return noInternetEgressRules(vpc, s3.Id), nil
}

func noInternetEgressRules(vpc *awsEc2.LookupVpcResult, s3PrefixListID string) awsEc2.SecurityGroupEgressArray {
	cidrs := []string{vpc.CidrBlock}
	for _, association := range vpc.CidrBlockAssociations {
		if !slices.Contains(cidrs, association.CidrBlock) {
			cidrs = append(cidrs, association.CidrBlock)
		}
	}
	return awsEc2.SecurityGroupEgressArray{
		awsEc2.SecurityGroupEgressArgs{
			CidrBlocks: pulumi.ToStringArray(cidrs),
			FromPort:   pulumi.Int(0),
			ToPort:     pulumi.Int(0),
			Protocol:   pulumi.String("-1"),
		},
		awsEc2.SecurityGroupEgressArgs{
			PrefixListIds: pulumi.StringArray{pulumi.String(s3PrefixListID)},
			FromPort:      pulumi.Int(443),
			ToPort:        pulumi.Int(443),
			Protocol:      pulumi.String("tcp"),
		},
	}
}
