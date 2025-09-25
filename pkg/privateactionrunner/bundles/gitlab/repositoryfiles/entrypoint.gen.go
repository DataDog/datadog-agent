// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repository_files

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabRepositoryFilesBundle struct{}

func NewGitlabRepositoryFiles() types.Bundle {
	return &GitlabRepositoryFilesBundle{}
}

func (b *GitlabRepositoryFilesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	// Manual actions
	case "getRawFile":
		return b.RunGetRawFile(ctx, task, credential)
	// Auto-generated actions
	case "createFile":
		return b.RunCreateFile(ctx, task, credential)
	case "deleteFile":
		return b.RunDeleteFile(ctx, task, credential)
	case "getFile":
		return b.RunGetFile(ctx, task, credential)
	case "getFileBlame":
		return b.RunGetFileBlame(ctx, task, credential)
	case "getFileMetaData":
		return b.RunGetFileMetaData(ctx, task, credential)
	case "updateFile":
		return b.RunUpdateFile(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *GitlabRepositoryFilesBundle) GetAction(actionName string) types.Action {
	return h
}
