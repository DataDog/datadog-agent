// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_repository_files

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabRepositoryFilesBundle struct {
	actions map[string]types.Action
}

func NewGitlabRepositoryFiles() types.Bundle {
	return &GitlabRepositoryFilesBundle{
		actions: map[string]types.Action{
			// Manual actions
			"getRawFile": NewGetRawFileHandler(),
			// Auto-generated actions
			"createFile":      NewCreateFileHandler(),
			"deleteFile":      NewDeleteFileHandler(),
			"getFile":         NewGetFileHandler(),
			"getFileBlame":    NewGetFileBlameHandler(),
			"getFileMetaData": NewGetFileMetaDataHandler(),
			"updateFile":      NewUpdateFileHandler(),
		},
	}
}

func (h *GitlabRepositoryFilesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
