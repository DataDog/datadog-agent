// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_notes

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteIssueNoteHandler struct{}

func NewDeleteIssueNoteHandler() *DeleteIssueNoteHandler {
	return &DeleteIssueNoteHandler{}
}

type DeleteIssueNoteInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	IssueIid  int64            `json:"issue_iid,omitempty"`
	NoteId    int64            `json:"note_id,omitempty"`
}

type DeleteIssueNoteOutputs struct{}

func (h *DeleteIssueNoteHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeleteIssueNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Notes.DeleteIssueNote(inputs.ProjectId.String(), inputs.IssueIid, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &DeleteIssueNoteOutputs{}, nil
}
