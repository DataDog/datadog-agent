// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type UpdateIssueNoteHandler struct{}

func NewUpdateIssueNoteHandler() *UpdateIssueNoteHandler {
	return &UpdateIssueNoteHandler{}
}

type UpdateIssueNoteInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
	NoteId    int          `json:"note_id,omitempty"`
	*gitlab.UpdateIssueNoteOptions
}

type UpdateIssueNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *UpdateIssueNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateIssueNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.UpdateIssueNote(inputs.ProjectId.String(), inputs.IssueIid, inputs.NoteId, inputs.UpdateIssueNoteOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateIssueNoteOutputs{Note: note}, nil
}
