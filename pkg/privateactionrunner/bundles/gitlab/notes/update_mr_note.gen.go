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

type UpdateMergeRequestNoteHandler struct{}

func NewUpdateMergeRequestNoteHandler() *UpdateMergeRequestNoteHandler {
	return &UpdateMergeRequestNoteHandler{}
}

type UpdateMergeRequestNoteInputs struct {
	ProjectId       support.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int64            `json:"merge_request_iid,omitempty"`
	NoteId          int64            `json:"note_id,omitempty"`
	*gitlab.UpdateMergeRequestNoteOptions
}

type UpdateMergeRequestNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *UpdateMergeRequestNoteHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[UpdateMergeRequestNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.UpdateMergeRequestNote(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.NoteId, inputs.UpdateMergeRequestNoteOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateMergeRequestNoteOutputs{Note: note}, nil
}
