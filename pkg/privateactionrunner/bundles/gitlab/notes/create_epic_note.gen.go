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

type CreateEpicNoteInputs struct {
	GroupId lib.GitlabID `json:"group_id,omitempty"`
	EpicId  int          `json:"epic_id,omitempty"`
	*gitlab.CreateEpicNoteOptions
}

type CreateEpicNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (b *GitlabNotesBundle) RunCreateEpicNote(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CreateEpicNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.CreateEpicNote(inputs.GroupId.String(), inputs.EpicId, inputs.CreateEpicNoteOptions)
	if err != nil {
		return nil, err
	}
	return &CreateEpicNoteOutputs{Note: note}, nil
}
