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

type ListMergeRequestNotesInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.ListMergeRequestNotesOptions
}

type ListMergeRequestNotesOutputs struct {
	Notes []*gitlab.Note `json:"notes"`
}

func (b *GitlabNotesBundle) RunListMergeRequestNotes(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListMergeRequestNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	notes, _, err := git.Notes.ListMergeRequestNotes(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.ListMergeRequestNotesOptions)
	if err != nil {
		return nil, err
	}
	return &ListMergeRequestNotesOutputs{Notes: notes}, nil
}
