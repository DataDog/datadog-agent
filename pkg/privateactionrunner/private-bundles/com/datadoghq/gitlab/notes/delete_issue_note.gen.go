package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteIssueNoteHandler struct{}

func NewDeleteIssueNoteHandler() *DeleteIssueNoteHandler {
	return &DeleteIssueNoteHandler{}
}

type DeleteIssueNoteInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	IssueIid  int          `json:"issue_iid,omitempty"`
	NoteId    int          `json:"note_id,omitempty"`
}

type DeleteIssueNoteOutputs struct{}

func (h *DeleteIssueNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteIssueNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Notes.DeleteIssueNote(inputs.ProjectId.String(), inputs.IssueIid, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &DeleteIssueNoteOutputs{}, nil
}
