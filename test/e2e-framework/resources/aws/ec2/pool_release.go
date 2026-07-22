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

// ScheduleReleaseOnDestroy attaches instanceID/leaseToken's release-and-restore
// logic to opts' owning stack via a local.Command whose Delete handler runs
// pool.BuildReleaseScript. Create is a no-op: this resource exists purely to
// carry a Delete action, since `pulumi destroy` never re-invokes the Go
// provisioner program that created it — cleanup can only happen through a
// resource's own provider-level Delete, not through Go code run after the fact.
//
// leaseToken is passed via Triggers, matching ReplaceRootVolumeToLaunchState's
// pattern, so a new lease on the same instance always produces a fresh trigger
// value even though instanceID doesn't change between cycles.
func ScheduleReleaseOnDestroy(e aws.Environment, name string, instanceID string, leaseToken string, opts ...pulumi.ResourceOption) (*local.Command, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceID is required to schedule a pool release")
	}
	if leaseToken == "" {
		return nil, fmt.Errorf("leaseToken is required to schedule a pool release")
	}

	return local.NewCommand(e.Ctx(), e.Namer.ResourceName(name), &local.CommandArgs{
		Create:      pulumi.String("true"),
		Delete:      pulumi.String(pool.BuildReleaseScript(instanceID, leaseToken)),
		Environment: awsCommandEnvironment(e),
		Triggers:    pulumi.Array{pulumi.String(leaseToken)},
	}, opts...)
}

// ScheduleReleaseOnDestroyOutput is ScheduleReleaseOnDestroy for a brand-new pool
// member: instanceID/deleteScript aren't known until after the EC2 instance is
// created (deleteScript embeds the lease token returned by registering the new
// member), so both are taken as Outputs instead of plain strings.
func ScheduleReleaseOnDestroyOutput(e aws.Environment, name string, instanceID pulumi.StringOutput, deleteScript pulumi.StringOutput, opts ...pulumi.ResourceOption) (*local.Command, error) {
	return local.NewCommand(e.Ctx(), e.Namer.ResourceName(name), &local.CommandArgs{
		Create:      pulumi.String("true"),
		Delete:      deleteScript.ToStringPtrOutput(),
		Environment: awsCommandEnvironment(e),
		Triggers:    pulumi.Array{instanceID},
	}, opts...)
}

// awsCommandEnvironment builds the env vars a local.Command needs to run AWS
// CLI calls against e's account/region. AWS_PROFILE is omitted when e.Profile()
// is empty (e.g. aws-vault-style credential env vars rather than a named
// profile) — passing AWS_PROFILE="" explicitly makes the AWS CLI look for a
// profile literally named "", which fails with "config profile () could not
// be found" even though credentials are already present in the environment.
func awsCommandEnvironment(e aws.Environment) pulumi.StringMap {
	env := pulumi.StringMap{
		"AWS_REGION": pulumi.String(e.Region()),
	}
	if profile := e.Profile(); profile != "" {
		env["AWS_PROFILE"] = pulumi.String(profile)
	}
	return env
}
