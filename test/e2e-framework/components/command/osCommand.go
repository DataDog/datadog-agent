// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"runtime"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// OSCommand defines the commands which are OS specifics
type OSCommand interface {
	GetTemporaryDirectory() string
	GetHomeDirectory() string

	CreateDirectory(
		runner Runner,
		resourceName string,
		remotePath pulumi.StringInput,
		useSudo bool,
		opts ...pulumi.ResourceOption) (Command, error)
	MoveFile(runner Runner, name string, source, destination pulumi.StringInput, sudo bool, opts ...pulumi.ResourceOption) (Command, error)

	BuildCommandString(
		command pulumi.StringInput,
		env pulumi.StringMap,
		sudo bool,
		passwordFromStdin bool,
		user string) pulumi.StringInput

	IsPathAbsolute(path string) bool
	PathJoin(parts ...string) string

	NewCopyFile(runner Runner, name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
	NewCopyToRemoteFile(runner Runner, name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
	copyLocalFile(runner *LocalRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
	copyRemoteFile(runner *RemoteRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
	// copyToRemoteFileV2 rely on CopyToRemote to copy files to remote, which uses a File asset instead of a Pulumi.StringInput with the path. It breaks when the path is not determined at runtime.
	copyRemoteFileV2(runner *RemoteRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
}

// ------------------------------
// Helpers to implement osCommand
// ------------------------------

const backupExtension = "pulumi.backup"

func createDirectory(
	runner Runner,
	name string,
	createCmd string,
	deleteCmd string,
	useSudo bool,
	opts ...pulumi.ResourceOption,
) (Command, error) {
	// If the folder was previously created, make sure to delete it before creating it.
	opts = append(opts, pulumi.DeleteBeforeReplace(true))
	return runner.Command(name,
		&Args{
			Create:   pulumi.String(createCmd),
			Delete:   pulumi.String(deleteCmd),
			Sudo:     useSudo,
			Triggers: pulumi.Array{pulumi.String(createCmd), pulumi.BoolPtr(useSudo)},
		}, opts...)
}

func buildCommandString(
	command pulumi.StringInput,
	envVars pulumi.StringArray,
	fct func(envVarsStr pulumi.StringOutput) pulumi.StringInput,
) pulumi.StringInput {
	if command == nil {
		return nil
	}
	envVarsStr := envVars.ToStringArrayOutput().ApplyT(func(inputs []string) string {
		return strings.Join(inputs, " ")
	}).(pulumi.StringOutput)

	return fct(envVarsStr)
}

func copyRemoteFile(
	runner Runner,
	name string,
	createCommand pulumi.StringInput,
	deleteCommand pulumi.StringInput,
	useSudo bool,
	opts ...pulumi.ResourceOption,
) (Command, error) {
	return runner.Command(name,
		&Args{
			Create:   createCommand,
			Delete:   deleteCommand,
			Sudo:     useSudo,
			Triggers: pulumi.Array{createCommand, deleteCommand, pulumi.BoolPtr(useSudo)},
		}, opts...)
}

func NewLocalOSCommand() OSCommand {
	if runtime.GOOS == "windows" {
		return NewWindowsOSCommand()
	}

	return NewUnixOSCommand()
}
