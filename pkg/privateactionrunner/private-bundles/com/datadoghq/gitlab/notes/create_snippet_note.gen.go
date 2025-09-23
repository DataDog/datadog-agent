// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CreateSnippetNoteHandler struct{}

func NewCreateSnippetNoteHandler() *CreateSnippetNoteHandler {
	return &CreateSnippetNoteHandler{}
}

type CreateSnippetNoteInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	SnippetId int          `json:"snippet_id,omitempty"`
	*gitlab.CreateSnippetNoteOptions
}

type CreateSnippetNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *CreateSnippetNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateSnippetNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.CreateSnippetNote(inputs.ProjectId.String(), inputs.SnippetId, inputs.CreateSnippetNoteOptions)
	if err != nil {
		return nil, err
	}
	return &CreateSnippetNoteOutputs{Note: note}, nil
}
