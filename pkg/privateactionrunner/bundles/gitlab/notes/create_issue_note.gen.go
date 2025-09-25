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

type CreateIssueNoteInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
	*gitlab.CreateIssueNoteOptions
}

type CreateIssueNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (b *GitlabNotesBundle) RunCreateIssueNote(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateIssueNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.CreateIssueNote(inputs.ProjectId.String(), inputs.IssueIid, inputs.CreateIssueNoteOptions)
	if err != nil {
		return nil, err
	}
	return &CreateIssueNoteOutputs{Note: note}, nil
}
