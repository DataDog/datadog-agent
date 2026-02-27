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

type CreateDraftNoteHandler struct{}

func NewCreateDraftNoteHandler() *CreateDraftNoteHandler {
	return &CreateDraftNoteHandler{}
}

type CreateDraftNoteInputs struct {
	ProjectId       support.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int64            `json:"merge_request_iid,omitempty"`
	*gitlab.CreateDraftNoteOptions
}

type CreateDraftNoteOutputs struct {
	DraftNote *gitlab.DraftNote `json:"draft_note"`
}

func (h *CreateDraftNoteHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateDraftNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	draftNote, _, err := git.DraftNotes.CreateDraftNote(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.CreateDraftNoteOptions)
	if err != nil {
		return nil, err
	}
	return &CreateDraftNoteOutputs{DraftNote: draftNote}, nil
}
