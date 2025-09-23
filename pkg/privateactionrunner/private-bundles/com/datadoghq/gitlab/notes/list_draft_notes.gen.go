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

type ListDraftNotesHandler struct{}

func NewListDraftNotesHandler() *ListDraftNotesHandler {
	return &ListDraftNotesHandler{}
}

type ListDraftNotesInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.ListDraftNotesOptions
}

type ListDraftNotesOutputs struct {
	DraftNotes []*gitlab.DraftNote `json:"draft_notes"`
}

func (h *ListDraftNotesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListDraftNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	draftNotes, _, err := git.DraftNotes.ListDraftNotes(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.ListDraftNotesOptions)
	if err != nil {
		return nil, err
	}
	return &ListDraftNotesOutputs{DraftNotes: draftNotes}, nil
}
