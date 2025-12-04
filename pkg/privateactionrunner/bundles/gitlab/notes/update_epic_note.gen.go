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

type UpdateEpicNoteHandler struct{}

func NewUpdateEpicNoteHandler() *UpdateEpicNoteHandler {
	return &UpdateEpicNoteHandler{}
}

type UpdateEpicNoteInputs struct {
	GroupId support.GitlabID `json:"group_id,omitempty"`
	EpicId  int64            `json:"epic_id,omitempty"`
	NoteId  int64            `json:"note_id,omitempty"`
	*gitlab.UpdateEpicNoteOptions
}

type UpdateEpicNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *UpdateEpicNoteHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[UpdateEpicNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.UpdateEpicNote(inputs.GroupId.String(), inputs.EpicId, inputs.NoteId, inputs.UpdateEpicNoteOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateEpicNoteOutputs{Note: note}, nil
}
