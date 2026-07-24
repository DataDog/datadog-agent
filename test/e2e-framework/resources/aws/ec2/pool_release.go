// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ScheduleReleaseOnDestroy attaches instanceID/leaseToken/imageID's release-and-revert
// logic to opts' owning stack via a local.Command whose Delete handler runs
// pool.BuildReleaseScript. Create is a no-op; the resource exists to carry a Delete
// action. imageID may be empty, in which case the Delete handler skips the
// root-volume replacement but still releases the lease. leaseToken is passed via
// Triggers so a new lease on the same instance produces a fresh trigger value.
func ScheduleReleaseOnDestroy(e aws.Environment, name string, instanceID string, leaseToken string, imageID string, opts ...pulumi.ResourceOption) (*local.Command, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceID is required to schedule a pool release")
	}
	if leaseToken == "" {
		return nil, fmt.Errorf("leaseToken is required to schedule a pool release")
	}

	return local.NewCommand(e.Ctx(), e.Namer.ResourceName(name), &local.CommandArgs{
		Create:      pulumi.String("true"),
		Delete:      pulumi.String(pool.BuildReleaseScript(instanceID, leaseToken, imageID)),
		Environment: awsCommandEnvironment(e),
		Triggers:    pulumi.Array{pulumi.String(leaseToken)},
	}, opts...)
}

// awsCommandEnvironment builds the AWS_REGION/AWS_PROFILE env vars a local.Command
// needs. AWS_PROFILE is omitted when empty, since an explicit empty value makes the
// AWS CLI look for a profile literally named "".
func awsCommandEnvironment(e aws.Environment) pulumi.StringMap {
	env := pulumi.StringMap{
		"AWS_REGION": pulumi.String(e.Region()),
	}
	if profile := e.Profile(); profile != "" {
		env["AWS_PROFILE"] = pulumi.String(profile)
	}
	return env
}
