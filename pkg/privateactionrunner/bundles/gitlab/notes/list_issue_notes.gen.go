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

type ListIssueNotesHandler struct{}

func NewListIssueNotesHandler() *ListIssueNotesHandler {
	return &ListIssueNotesHandler{}
}

type ListIssueNotesInputs struct {
	ProjectId support.GitlabID `json:"project_id,omitempty"`
	IssueIid  int64            `json:"issue_iid,omitempty"`
	*gitlab.ListIssueNotesOptions
}

type ListIssueNotesOutputs struct {
	Notes []*gitlab.Note `json:"notes"`
}

func (h *ListIssueNotesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[ListIssueNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	notes, _, err := git.Notes.ListIssueNotes(inputs.ProjectId.String(), inputs.IssueIid, inputs.ListIssueNotesOptions)
	if err != nil {
		return nil, err
	}
	return &ListIssueNotesOutputs{Notes: notes}, nil
}
