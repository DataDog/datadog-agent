// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ec2

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ScheduleRegisterOnCreate attaches instanceID's golden-snapshot-and-register logic
// (pool.BuildRegisterScript) to opts' owning stack via a local.Command's Create
// handler. Call this only once, right after remote.InitHost succeeds for a freshly
// created (non-imported) local pool member.
func ScheduleRegisterOnCreate(e aws.Environment, name string, instanceID pulumi.StringOutput, ownerPipelineID, username string, opts ...pulumi.ResourceOption) (*local.Command, error) {
	script := instanceID.ApplyT(func(id string) string {
		return pool.BuildRegisterScript(id, ownerPipelineID, username)
	}).(pulumi.StringOutput)

	return local.NewCommand(e.Ctx(), e.Namer.ResourceName(name), &local.CommandArgs{
		Create:      script,
		Environment: awsCommandEnvironment(e),
		Triggers:    pulumi.Array{instanceID},
	}, opts...)
}

// awsCommandEnvironment builds the env vars a local.Command needs to run AWS CLI
// calls against e's account/region. AWS_PROFILE is omitted when e.Profile() is
// empty, since passing it as an empty string breaks AWS CLI profile resolution.
func awsCommandEnvironment(e aws.Environment) pulumi.StringMap {
	env := pulumi.StringMap{
		"AWS_REGION": pulumi.String(e.Region()),
	}
	if profile := e.Profile(); profile != "" {
		env["AWS_PROFILE"] = pulumi.String(profile)
	}
	return env
}
