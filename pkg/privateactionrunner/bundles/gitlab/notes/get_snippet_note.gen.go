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

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetSnippetNoteHandler struct{}

func NewGetSnippetNoteHandler() *GetSnippetNoteHandler {
	return &GetSnippetNoteHandler{}
}

type GetSnippetNoteInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	SnippetId int64            `json:"snippet_id,omitempty"`
	NoteId    int64            `json:"note_id,omitempty"`
}

type GetSnippetNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *GetSnippetNoteHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetSnippetNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.GetSnippetNote(inputs.ProjectId.String(), inputs.SnippetId, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &GetSnippetNoteOutputs{Note: note}, nil
}
