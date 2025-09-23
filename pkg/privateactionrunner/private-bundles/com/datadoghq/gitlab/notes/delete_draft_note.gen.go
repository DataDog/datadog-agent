// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteDraftNoteHandler struct{}

func NewDeleteDraftNoteHandler() *DeleteDraftNoteHandler {
	return &DeleteDraftNoteHandler{}
}

type DeleteDraftNoteInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	DraftNoteId     int          `json:"draft_note_id,omitempty"`
}

type DeleteDraftNoteOutputs struct{}

func (h *DeleteDraftNoteHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteDraftNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.DraftNotes.DeleteDraftNote(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.DraftNoteId)
	if err != nil {
		return nil, err
	}
	return &DeleteDraftNoteOutputs{}, nil
}
