// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteEpicNoteInputs struct {
	GroupId lib.GitlabID `json:"group_id,omitempty"`
	EpicId  int          `json:"epic_id,omitempty"`
	NoteId  int          `json:"note_id,omitempty"`
}

type DeleteEpicNoteOutputs struct{}

func (b *GitlabNotesBundle) RunDeleteEpicNote(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteEpicNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Notes.DeleteEpicNote(inputs.GroupId.String(), inputs.EpicId, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &DeleteEpicNoteOutputs{}, nil
}
