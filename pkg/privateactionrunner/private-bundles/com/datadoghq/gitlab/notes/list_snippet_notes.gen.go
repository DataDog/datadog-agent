package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListSnippetNotesHandler struct{}

func NewListSnippetNotesHandler() *ListSnippetNotesHandler {
	return &ListSnippetNotesHandler{}
}

type ListSnippetNotesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	SnippetId int          `json:"snippet_id,omitempty"`
	*gitlab.ListSnippetNotesOptions
}

type ListSnippetNotesOutputs struct {
	Notes []*gitlab.Note `json:"notes"`
}

func (h *ListSnippetNotesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListSnippetNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	notes, _, err := git.Notes.ListSnippetNotes(inputs.ProjectId.String(), inputs.SnippetId, inputs.ListSnippetNotesOptions)
	if err != nil {
		return nil, err
	}
	return &ListSnippetNotesOutputs{Notes: notes}, nil
}
