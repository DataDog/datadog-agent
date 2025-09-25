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

type ListSnippetNotesInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	SnippetId int          `json:"snippet_id,omitempty"`
	*gitlab.ListSnippetNotesOptions
}

type ListSnippetNotesOutputs struct {
	Notes []*gitlab.Note `json:"notes"`
}

func (b *GitlabNotesBundle) RunListSnippetNotes(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListSnippetNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	notes, _, err := git.Notes.ListSnippetNotes(inputs.ProjectId.String(), inputs.SnippetId, inputs.ListSnippetNotesOptions)
	if err != nil {
		return nil, err
	}
	return &ListSnippetNotesOutputs{Notes: notes}, nil
}
