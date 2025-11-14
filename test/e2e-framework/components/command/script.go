// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"path"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	baseRemotePath = "/tmp"
)

// If *Param is nil, the create/update/delete command is not run
func NewScript(ctx *pulumi.Context,
	conn *remote.ConnectionArgs,
	localScriptPath string,
	createParam, updateParam, deleteParam *string,
) (*remote.Command, error) {
	fileHash, err := utils.FileHash(localScriptPath)
	if err != nil {
		return nil, err
	}

	fileName := path.Base(localScriptPath)
	remotePath := path.Join(baseRemotePath, "remote-"+fileHash, fileName)

	copyFile, err := remote.NewCopyFile(ctx, fileName+"-"+fileHash, &remote.CopyFileArgs{
		Connection: conn,
		LocalPath:  pulumi.String(localScriptPath),
		RemotePath: pulumi.String(remotePath),
	})
	if err != nil {
		return nil, err
	}

	runCommandArgs := &remote.CommandArgs{
		Connection: conn,
	}
	if createParam != nil {
		runCommandArgs.Create = pulumi.StringPtr(remotePath + " " + *createParam)
	}
	if updateParam != nil {
		runCommandArgs.Update = pulumi.StringPtr(remotePath + " " + *updateParam)
	}
	if deleteParam != nil {
		runCommandArgs.Delete = pulumi.StringPtr(remotePath + " " + *deleteParam)
	}

	return remote.NewCommand(ctx, "remote-"+fileHash, runCommandArgs, pulumi.DependsOn([]pulumi.Resource{copyFile}))
}
