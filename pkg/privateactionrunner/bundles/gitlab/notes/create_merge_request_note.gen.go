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

type CreateMergeRequestNoteHandler struct{}

func NewCreateMergeRequestNoteHandler() *CreateMergeRequestNoteHandler {
	return &CreateMergeRequestNoteHandler{}
}

type CreateMergeRequestNoteInputs struct {
	ProjectId       support.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int64            `json:"merge_request_iid,omitempty"`
	*gitlab.CreateMergeRequestNoteOptions
}

type CreateMergeRequestNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (h *CreateMergeRequestNoteHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateMergeRequestNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.CreateMergeRequestNote(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.CreateMergeRequestNoteOptions)
	if err != nil {
		return nil, err
	}
	return &CreateMergeRequestNoteOutputs{Note: note}, nil
}
