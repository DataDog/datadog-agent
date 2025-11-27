// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var _ OSCommand = (*windowsOSCommand)(nil)

type windowsOSCommand struct{}

func NewWindowsOSCommand() OSCommand {
	return windowsOSCommand{}
}

// CreateDirectory if it does not exist
func (fs windowsOSCommand) CreateDirectory(
	runner Runner,
	name string,
	remotePath pulumi.StringInput,
	_ bool,
	opts ...pulumi.ResourceOption,
) (Command, error) {
	useSudo := false
	return createDirectory(
		runner,
		name,
		fmt.Sprintf("New-Item -Force -Path %v -ItemType Directory", remotePath),
		fmt.Sprintf("if (-not (Test-Path -Path %v/*)) { Remove-Item -Path %v -ErrorAction SilentlyContinue }", remotePath, remotePath),
		useSudo,
		opts...)
}

func (fs windowsOSCommand) GetTemporaryDirectory() string {
	return "$env:TEMP"
}

func (fs windowsOSCommand) GetHomeDirectory() string {
	// %HOMEDRIVE% returns the disk drive where home directory is located
	// %HOMEPATH% returns the path to the home directory related to HOMEDRIVE
	return "$env:HOMEDRIVE$env:HOMEPATH"
}

func (fs windowsOSCommand) BuildCommandString(
	command pulumi.StringInput,
	env pulumi.StringMap,
	_ bool,
	_ bool,
	_ string,
) pulumi.StringInput {
	var envVars pulumi.StringArray
	for varName, varValue := range env {
		envVars = append(envVars, pulumi.Sprintf(`$env:%v = '%v'; `, varName, varValue))
	}

	return buildCommandString(command, envVars, func(envVarsStr pulumi.StringOutput) pulumi.StringInput {
		return pulumi.Sprintf("%s %s", envVarsStr, command)
	})
}

func (fs windowsOSCommand) PathJoin(parts ...string) string {
	return strings.Join(parts, "\\")
}

func (fs windowsOSCommand) IsPathAbsolute(path string) bool {
	// valid absolute path prefixes: "x:\", "x:/", "\\", "//" ]
	if len(path) < 2 {
		return false
	}
	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\\`) {
		return true
	} else if strings.Index(path, ":/") == 1 {
		return true
	} else if strings.Index(path, `:\`) == 1 {
		return true
	}
	return false
}

func (fs windowsOSCommand) NewCopyFile(runner Runner, name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return runner.newCopyFile(name, localPath, remotePath, opts...)
}

func (fs windowsOSCommand) NewCopyToRemoteFile(runner Runner, name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return runner.newCopyToRemoteFile(name, localPath, remotePath, opts...)
}

func (fs windowsOSCommand) MoveFile(runner Runner, name string, source, destination pulumi.StringInput, sudo bool, opts ...pulumi.ResourceOption) (Command, error) {
	backupPath := pulumi.Sprintf("%v.%s", destination, backupExtension)
	copyCommand := pulumi.Sprintf(`Copy-Item -Path '%v' -Destination '%v'`, source, destination)
	createCommand := pulumi.Sprintf(`if (Test-Path '%v') { Move-Item -Force -Path '%v' -Destination '%v' }; %v`, destination, destination, backupPath, copyCommand)
	deleteCommand := pulumi.Sprintf(`if (Test-Path '%v') { Move-Item -Force -Path '%v' -Destination '%v' } else { Remove-Item -Force -Path %v }`, backupPath, backupPath, destination, destination)
	return copyRemoteFile(runner, fmt.Sprintf("move-file-%s", name), createCommand, deleteCommand, sudo, utils.MergeOptions(opts, pulumi.ReplaceOnChanges([]string{"*"}), pulumi.DeleteBeforeReplace(true))...)
}

func (fs windowsOSCommand) copyLocalFile(runner *LocalRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	createCmd := pulumi.Sprintf("Copy-Item -Path '%v' -Destination '%v'", src, dst)
	deleteCmd := pulumi.Sprintf("Remove-Item -Path '%v'", dst)
	useSudo := false

	return runner.Command(name,
		&Args{
			Create:   createCmd,
			Delete:   deleteCmd,
			Sudo:     useSudo,
			Triggers: pulumi.Array{createCmd, deleteCmd, pulumi.BoolPtr(useSudo)},
		}, opts...)
}

func (fs windowsOSCommand) copyRemoteFile(runner *RemoteRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return remote.NewCopyFile(runner.Environment().Ctx(), runner.Namer().ResourceName("copy", name), &remote.CopyFileArgs{
		Connection: runner.Config().connection,
		LocalPath:  src,
		RemotePath: dst,
		Triggers:   pulumi.Array{src, dst},
	}, utils.MergeOptions(runner.PulumiOptions(), opts...)...)
}

func (fs windowsOSCommand) copyRemoteFileV2(runner *RemoteRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	srcAsset := src.ToStringOutput().ApplyT(func(path string) pulumi.AssetOrArchive {
		return pulumi.NewFileAsset(path)
	}).(pulumi.AssetOrArchiveOutput)

	return remote.NewCopyToRemote(runner.Environment().Ctx(), runner.Namer().ResourceName("copy", name), &remote.CopyToRemoteArgs{
		Connection: runner.Config().connection,
		Source:     srcAsset,
		RemotePath: dst,
		Triggers:   pulumi.Array{src, dst},
	}, utils.MergeOptions(runner.PulumiOptions(), opts...)...)
}
