// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_issues

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetIssueByIDHandler struct{}

func NewGetIssueByIDHandler() *GetIssueByIDHandler {
	return &GetIssueByIDHandler{}
}

type GetIssueByIDInputs struct {
	IssueIid int `json:"issue_iid,omitempty"`
}

type GetIssueByIDOutputs struct {
	Issue *gitlab.Issue `json:"issue"`
}

func (h *GetIssueByIDHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetIssueByIDInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	issue, _, err := git.Issues.GetIssueByID(inputs.IssueIid)
	if err != nil {
		return nil, err
	}
	return &GetIssueByIDOutputs{Issue: issue}, nil
}
