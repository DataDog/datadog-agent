package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateDraftNoteHandler struct{}

func NewCreateDraftNoteHandler() *CreateDraftNoteHandler {
	return &CreateDraftNoteHandler{}
}

type CreateDraftNoteInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.CreateDraftNoteOptions
}

type CreateDraftNoteOutputs struct {
	DraftNote *gitlab.DraftNote `json:"draft_note"`
}

func (h *CreateDraftNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateDraftNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	draftNote, _, err := git.DraftNotes.CreateDraftNote(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.CreateDraftNoteOptions)
	if err != nil {
		return nil, err
	}
	return &CreateDraftNoteOutputs{DraftNote: draftNote}, nil
}
