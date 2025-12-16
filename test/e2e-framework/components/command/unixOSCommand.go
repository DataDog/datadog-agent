// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"

	"github.com/alessio/shellescape"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	linuxTempDir = "/tmp"
	linuxHomeDir = "$HOME"
)

var _ OSCommand = (*unixOSCommand)(nil)

type unixOSCommand struct{}

func NewUnixOSCommand() OSCommand {
	return unixOSCommand{}
}

// CreateDirectory if it does not exist
func (unixOSCommand) CreateDirectory(
	runner Runner,
	name string,
	remotePath pulumi.StringInput,
	useSudo bool,
	opts ...pulumi.ResourceOption,
) (Command, error) {
	createCmd := fmt.Sprintf("mkdir -p %v", remotePath)
	deleteCmd := fmt.Sprintf(`bash -c 'if [ -z "$(ls -A %v)" ]; then rm -d %v; fi'`, remotePath, remotePath)
	// check if directory already exist
	return createDirectory(
		runner,
		name,
		createCmd,
		deleteCmd,
		useSudo,
		opts...)
}

func (fs unixOSCommand) GetTemporaryDirectory() string {
	return linuxTempDir
}

func (fs unixOSCommand) GetHomeDirectory() string {
	return linuxHomeDir
}

// BuildCommandString properly format the command string
// command can be nil
func (fs unixOSCommand) BuildCommandString(command pulumi.StringInput, env pulumi.StringMap, sudo bool, password bool, user string) pulumi.StringInput {
	formattedCommand := formatCommandIfNeeded(command, sudo, password, user)

	var envVars pulumi.StringArray
	for varName, varValue := range env {
		envVars = append(envVars, pulumi.Sprintf(`export %v="%v";`, varName, varValue))
	}

	return buildCommandString(formattedCommand, envVars, func(envVarsStr pulumi.StringOutput) pulumi.StringInput {
		return pulumi.Sprintf("%s %s", envVarsStr, formattedCommand)
	})
}

func (fs unixOSCommand) PathJoin(parts ...string) string {
	return strings.Join(parts, "/")
}

func (fs unixOSCommand) IsPathAbsolute(path string) bool {
	return strings.HasPrefix(path, "/")
}

func (fs unixOSCommand) NewCopyFile(runner Runner, name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return runner.newCopyFile(name, localPath, remotePath, opts...)
}

func (fs unixOSCommand) NewCopyToRemoteFile(runner Runner, name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return runner.newCopyToRemoteFile(name, localPath, remotePath, opts...)
}

func formatCommandIfNeeded(command pulumi.StringInput, sudo bool, password bool, user string) pulumi.StringInput {
	if command == nil {
		return nil
	}

	if !sudo && user == "" {
		return command
	}
	var formattedCommand pulumi.StringInput
	if sudo && password {
		formattedCommand = pulumi.Sprintf("sudo -S %v", command)
	} else if sudo {
		formattedCommand = pulumi.Sprintf("sudo %v", command)
	} else if user != "" {
		formattedCommand = command.ToStringOutput().ApplyT(func(cmd string) string {
			return fmt.Sprintf("sudo -u %v bash -c %v", user, shellescape.Quote(cmd))
		}).(pulumi.StringOutput)
	}
	return formattedCommand
}

func (fs unixOSCommand) MoveFile(runner Runner, name string, source, destination pulumi.StringInput, sudo bool, opts ...pulumi.ResourceOption) (Command, error) {
	backupPath := pulumi.Sprintf("%v.%s", destination, backupExtension)
	copyCommand := pulumi.Sprintf(`cp '%v' '%v'`, source, destination)
	createCommand := pulumi.Sprintf(`bash -c 'if [ -f '%v' ]; then mv -f '%v' '%v'; fi; %v'`, destination, destination, backupPath, copyCommand)
	deleteCommand := pulumi.Sprintf(`bash -c 'if [ -f '%v' ]; then mv -f '%v' '%v'; else rm -f '%v'; fi'`, backupPath, backupPath, destination, destination)
	return copyRemoteFile(runner, fmt.Sprintf("move-file-%s", name), createCommand, deleteCommand, sudo, utils.MergeOptions(opts, pulumi.ReplaceOnChanges([]string{"*"}), pulumi.DeleteBeforeReplace(true))...)
}

func (fs unixOSCommand) copyLocalFile(runner *LocalRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	createCmd := pulumi.Sprintf("cp '%v' '%v'", src, dst)
	deleteCmd := pulumi.Sprintf("rm '%v'", dst)
	useSudo := false

	return runner.Command(name,
		&Args{
			Create:   createCmd,
			Delete:   deleteCmd,
			Sudo:     useSudo,
			Triggers: pulumi.Array{createCmd, deleteCmd, pulumi.BoolPtr(useSudo)},
		}, opts...)
}

func (fs unixOSCommand) copyRemoteFile(runner *RemoteRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	tempRemotePath := src.ToStringOutput().ApplyT(func(path string) string {
		return fs.PathJoin(runner.OsCommand().GetTemporaryDirectory(), filepath.Base(path))
	}).(pulumi.StringOutput)

	tempCopyFile, err := remote.NewCopyFile(runner.Environment().Ctx(), runner.Namer().ResourceName("copy", name), &remote.CopyFileArgs{
		Connection: runner.Config().connection,
		LocalPath:  src,
		RemotePath: tempRemotePath,
		Triggers:   pulumi.Array{src, tempRemotePath},
	}, utils.MergeOptions(runner.PulumiOptions(), opts...)...)

	if err != nil {
		return nil, err
	}

	moveCommand, err := runner.OsCommand().MoveFile(runner, name, tempRemotePath, dst, true, utils.MergeOptions(opts, utils.PulumiDependsOn(tempCopyFile))...)
	if err != nil {
		return nil, err
	}

	return moveCommand, err
}

func (fs unixOSCommand) copyRemoteFileV2(runner *RemoteRunner, name string, src, dst pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
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
