// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListEpicNotesInputs struct {
	GroupId lib.GitlabID `json:"group_id,omitempty"`
	EpicId  int          `json:"epic_id,omitempty"`
	*gitlab.ListEpicNotesOptions
}

type ListEpicNotesOutputs struct {
	Notes []*gitlab.Note `json:"notes"`
}

func (b *GitlabNotesBundle) RunListEpicNotes(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListEpicNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	notes, _, err := git.Notes.ListEpicNotes(inputs.GroupId.String(), inputs.EpicId, inputs.ListEpicNotesOptions)
	if err != nil {
		return nil, err
	}
	return &ListEpicNotesOutputs{Notes: notes}, nil
}
