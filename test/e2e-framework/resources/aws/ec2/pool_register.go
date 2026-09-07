// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ec2

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// poolMemberTagSlugs names the tag resources. Tag keys contain ':', which is the Pulumi
// URN delimiter, so resource names use these slugs instead of the raw keys.
var poolMemberTagSlugs = []struct {
	slug string
	key  string
}{
	{"pool-instance", pool.PoolTagKey},
	{"pool-owner", pool.OwnerUsernameTagKey},
	{"pool-name", "Name"},
}

// RegisterPoolMember bakes instanceID's current disk state into a golden AMI and tags it
// as a pool member owned by username, returning the AMI ID. The lease record is published
// separately by the test harness, since it is mutated outside Pulumi's control.
//
// Call this exactly once, for a freshly created instance, and pass opts that order it
// after remote.InitHost so the AMI captures the finished setup rather than a bare OS.
func RegisterPoolMember(e aws.Environment, name string, instanceID pulumi.StringOutput, username string, opts ...pulumi.ResourceOption) (pulumi.StringOutput, error) {
	ami, err := ec2.NewAmiFromInstance(e.Ctx(), e.Namer.ResourceName(name, "baseline"),
		&ec2.AmiFromInstanceArgs{
			SourceInstanceId: instanceID,
			Name:             pulumi.Sprintf("macos-e2e-pool-%s-%s", username, instanceID),
			// Without this the provider stops the instance to snapshot it, which would
			// drop the SSH connection InitHost just established. The trade-off is a
			// baseline captured from a live filesystem.
			SnapshotWithoutReboot: pulumi.Bool(true),
		},
		// The AMI is the baseline every later run reverts to, so it must outlive the
		// stack that created it.
		utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS), pulumi.RetainOnDelete(true))...)
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	values := map[string]string{
		pool.PoolTagKey:          pool.PoolTagValue,
		pool.OwnerUsernameTagKey: username,
		"Name":                   "macos-e2e-pool-" + username,
	}
	for _, t := range poolMemberTagSlugs {
		if _, err := ec2.NewTag(e.Ctx(), e.Namer.ResourceName(name, t.slug), &ec2.TagArgs{
			ResourceId: instanceID,
			Key:        pulumi.String(t.key),
			Value:      pulumi.String(values[t.key]),
		}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS))...); err != nil {
			return pulumi.StringOutput{}, err
		}
	}

	return ami.ID().ToStringOutput(), nil
}
