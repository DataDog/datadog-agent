// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// InstanceArgs defines the parameters for creating an EC2 instance.
type InstanceArgs struct {
	// Mandatory
	AMI string

	// Defaulted
	InstanceType    string // Note that caller must ensure it matches with AMI Architecture
	KeyPairName     string
	Tenancy         string
	StorageSize     int
	InstanceProfile string

	// Optional
	UserData           string
	HTTPTokensRequired bool
	HostID             pulumi.StringInput // For dedicated host tenancy
	VolumeThroughput   int                // GP3 volume throughput in MiB/s (125-1000)
}

// NewInstance creates a new EC2 instance with the given parameters.
func NewInstance(e aws.Environment, name string, args InstanceArgs, opts ...pulumi.ResourceOption) (*ec2.Instance, error) {
	defaultInstanceArgs(e, &args)

	rootBlockDevice := ec2.InstanceRootBlockDeviceArgs{
		VolumeSize: pulumi.Int(args.StorageSize),
	}
	if args.VolumeThroughput > 0 {
		// Only GP3 volumes support throughput
		rootBlockDevice.VolumeType = pulumi.String("gp3")
		rootBlockDevice.Throughput = pulumi.Int(args.VolumeThroughput)
	}

	instanceArgs := &ec2.InstanceArgs{
		Ami:                     pulumi.StringPtr(args.AMI),
		SubnetId:                e.RandomSubnets().Index(pulumi.Int(0)),
		IamInstanceProfile:      pulumi.StringPtr(args.InstanceProfile),
		InstanceType:            pulumi.StringPtr(args.InstanceType),
		VpcSecurityGroupIds:     pulumi.ToStringArray(e.DefaultSecurityGroups()),
		KeyName:                 pulumi.StringPtr(args.KeyPairName),
		UserData:                pulumi.StringPtr(args.UserData),
		UserDataReplaceOnChange: pulumi.BoolPtr(true),
		Tenancy:                 pulumi.StringPtr(args.Tenancy),
		RootBlockDevice:         rootBlockDevice,
		Tags: pulumi.StringMap{
			"Name": e.Namer.DisplayName(255, pulumi.String(name)),
		},
		InstanceInitiatedShutdownBehavior: pulumi.String(e.DefaultShutdownBehavior()),
		HostId:                            args.HostID,
	}

	if args.HTTPTokensRequired {
		instanceArgs.MetadataOptions = &ec2.InstanceMetadataOptionsArgs{
			HttpTokens: pulumi.String("required"),
		}
	}

	instance, err := ec2.NewInstance(e.Ctx(), e.Namer.ResourceName(name), instanceArgs, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS))...)

	return instance, err
}

func defaultInstanceArgs(e aws.Environment, args *InstanceArgs) {
	if args.InstanceType == "" {
		args.InstanceType = e.DefaultInstanceType()
	}
	if args.KeyPairName == "" {
		args.KeyPairName = e.DefaultKeyPairName()
	}
	if args.Tenancy == "" {
		args.Tenancy = string(ec2.TenancyDefault)
	}
	if args.StorageSize == 0 {
		args.StorageSize = e.DefaultInstanceStorageSize()
	}
}
