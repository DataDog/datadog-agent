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

type ListEpicNotesHandler struct{}

func NewListEpicNotesHandler() *ListEpicNotesHandler {
	return &ListEpicNotesHandler{}
}

type ListEpicNotesInputs struct {
	GroupId support.GitlabID `json:"group_id,omitempty"`
	EpicId  int64            `json:"epic_id,omitempty"`
	*gitlab.ListEpicNotesOptions
}

type ListEpicNotesOutputs struct {
	Notes []*gitlab.Note `json:"notes"`
}

func (h *ListEpicNotesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListEpicNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	notes, _, err := git.Notes.ListEpicNotes(inputs.GroupId.String(), inputs.EpicId, inputs.ListEpicNotesOptions)
	if err != nil {
		return nil, err
	}
	return &ListEpicNotesOutputs{Notes: notes}, nil
}
