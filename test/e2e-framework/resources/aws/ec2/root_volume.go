// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ReplaceRootVolumeToLaunchState resets instanceID's root volume to its exact
// state at launch time (no AMI/snapshot involved), via
// `aws ec2 create-replace-root-volume-task`. The instance must be running; it
// is rebooted automatically as part of the task, which also clears RAM.
//
// leaseToken is passed via Triggers so the command re-runs on every lease
// cycle even though instanceID/deleteReplacedVolume don't change between
// cycles — pulumi-command only re-executes Create when a trigger value
// changes, and every lease is a fresh trigger value.
//
// There is no built-in `aws ec2 wait` waiter for this task type (only
// volume-available/-deleted/-in-use exist, i.e. plain EBS volume states, not
// this task), so completion is polled manually via
// describe-replace-root-volume-tasks.
func ReplaceRootVolumeToLaunchState(e aws.Environment, name string, instanceID string, deleteReplacedVolume bool, leaseToken string, opts ...pulumi.ResourceOption) (*local.Command, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceID is required to replace a root volume")
	}

	deleteFlag := ""
	if deleteReplacedVolume {
		deleteFlag = "--delete-replaced-root-volume"
	}

	// The AWS CLI is used here (rather than a raw aws-sdk-go-v2 client) purely
	// because local.Command shells out to a command string; the underlying API
	// call is identical either way.
	createCmd := fmt.Sprintf(
		`task_id=$(aws ec2 create-replace-root-volume-task --instance-id %s %s --query 'ReplaceRootVolumeTask.ReplaceRootVolumeTaskId' --output text) && `+
			`while [ "$(aws ec2 describe-replace-root-volume-tasks --replace-root-volume-task-ids "$task_id" --query 'ReplaceRootVolumeTasks[0].TaskState' --output text)" != "succeeded" ]; do sleep 10; done`,
		instanceID, deleteFlag,
	)

	return local.NewCommand(e.Ctx(), e.Namer.ResourceName(name), &local.CommandArgs{
		Create:      pulumi.String(createCmd),
		Environment: awsCommandEnvironment(e),
		Triggers:    pulumi.Array{pulumi.String(leaseToken)},
	}, opts...)
}
