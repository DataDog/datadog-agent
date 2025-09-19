package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetSnippetNoteHandler struct{}

func NewGetSnippetNoteHandler() *GetSnippetNoteHandler {
	return &GetSnippetNoteHandler{}
}

type GetSnippetNoteInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	SnippetId int          `json:"snippet_id,omitempty"`
	NoteId    int          `json:"note_id,omitempty"`
}

type GetSnippetNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *GetSnippetNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetSnippetNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.GetSnippetNote(inputs.ProjectId.String(), inputs.SnippetId, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &GetSnippetNoteOutputs{Note: note}, nil
}
