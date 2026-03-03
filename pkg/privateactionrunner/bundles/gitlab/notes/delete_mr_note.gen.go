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
)

type DeleteMergeRequestNoteHandler struct{}

func NewDeleteMergeRequestNoteHandler() *DeleteMergeRequestNoteHandler {
	return &DeleteMergeRequestNoteHandler{}
}

type DeleteMergeRequestNoteInputs struct {
	ProjectId       support.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int64            `json:"merge_request_iid,omitempty"`
	NoteId          int64            `json:"note_id,omitempty"`
}

type DeleteMergeRequestNoteOutputs struct{}

func (h *DeleteMergeRequestNoteHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[DeleteMergeRequestNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Notes.DeleteMergeRequestNote(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &DeleteMergeRequestNoteOutputs{}, nil
}
