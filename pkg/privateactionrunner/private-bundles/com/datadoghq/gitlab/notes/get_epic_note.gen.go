package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetEpicNoteHandler struct{}

func NewGetEpicNoteHandler() *GetEpicNoteHandler {
	return &GetEpicNoteHandler{}
}

type GetEpicNoteInputs struct {
	GroupId lib.GitlabID `json:"group_id,omitempty"`
	EpicId  int          `json:"epic_id,omitempty"`
	NoteId  int          `json:"note_id,omitempty"`
}

type GetEpicNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *GetEpicNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetEpicNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.GetEpicNote(inputs.GroupId.String(), inputs.EpicId, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &GetEpicNoteOutputs{Note: note}, nil
}
