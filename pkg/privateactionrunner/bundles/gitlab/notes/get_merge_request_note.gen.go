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

type GetMergeRequestNoteInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	NoteId          int          `json:"note_id,omitempty"`
}

type GetMergeRequestNoteOutputs struct {
	Note *gitlab.Note `json:"note"`
}

func (b *GitlabNotesBundle) RunGetMergeRequestNote(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetMergeRequestNoteInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	note, _, err := git.Notes.GetMergeRequestNote(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.NoteId)
	if err != nil {
		return nil, err
	}
	return &GetMergeRequestNoteOutputs{Note: note}, nil
}
