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

type UpdateSnippetNoteHandler struct{}

func NewUpdateSnippetNoteHandler() *UpdateSnippetNoteHandler {
	return &UpdateSnippetNoteHandler{}
}

type UpdateSnippetNoteInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	SnippetId int          `json:"snippet_id,omitempty"`
	NoteId    int          `json:"note_id,omitempty"`
	*gitlab.UpdateSnippetNoteOptions
}

type UpdateSnippetNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *UpdateSnippetNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateSnippetNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.UpdateSnippetNote(inputs.ProjectId.String(), inputs.SnippetId, inputs.NoteId, inputs.UpdateSnippetNoteOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateSnippetNoteOutputs{Note: note}, nil
}
