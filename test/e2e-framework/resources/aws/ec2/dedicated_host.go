// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type DedicatedHostArgs struct {
	// Mandatory
	InstanceType string // e.g., "mac1.metal", "mac2.metal"

	// Optional
	AvailabilityZone string // If not specified, will use first available zone
	HostRecovery     string // "on" or "off", defaults to "off"
	Tags             pulumi.StringMap
}

// NewDedicatedHost creates an EC2 Dedicated Host for Mac instances
func NewDedicatedHost(e aws.Environment, name string, args DedicatedHostArgs, opts ...pulumi.ResourceOption) (*ec2.DedicatedHost, error) {
	if args.InstanceType == "" {
		return nil, fmt.Errorf("InstanceType is required for dedicated host")
	}

	// Default values
	if args.HostRecovery == "" {
		args.HostRecovery = "off"
	}

	var availabilityZone pulumi.StringInput
	if args.AvailabilityZone == "" {
		// Use the same AZ as the first subnet
		availabilityZone = e.RandomSubnets().Index(pulumi.Int(0)).ApplyT(func(subnetId string) (string, error) {
			// Get subnet info to determine AZ
			subnet, err := ec2.LookupSubnet(e.Ctx(), &ec2.LookupSubnetArgs{
				Id: &subnetId,
			}, e.WithProvider(config.ProviderAWS))
			if err != nil {
				return "", err
			}
			return subnet.AvailabilityZone, nil
		}).(pulumi.StringOutput)
	} else {
		availabilityZone = pulumi.String(args.AvailabilityZone)
	}

	dedicatedHostArgs := &ec2.DedicatedHostArgs{
		InstanceType:     pulumi.String(args.InstanceType),
		AvailabilityZone: availabilityZone,
		HostRecovery:     pulumi.String(args.HostRecovery),
	}

	return ec2.NewDedicatedHost(e.Ctx(),
		e.Namer.ResourceName(name),
		dedicatedHostArgs,
		utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS), pulumi.RetainOnDelete(true))..., // Retain on delete because deleting a dedicated host is not possible unless it lived for at least 24 hours, the cleanup will be done by test-infra-cleaner
	)
}
