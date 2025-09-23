// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type CancelMergeWhenPipelineSucceedsHandler struct{}

func NewCancelMergeWhenPipelineSucceedsHandler() *CancelMergeWhenPipelineSucceedsHandler {
	return &CancelMergeWhenPipelineSucceedsHandler{}
}

type CancelMergeWhenPipelineSucceedsInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
}

type CancelMergeWhenPipelineSucceedsOutputs struct {
	MergeRequest *gitlab.MergeRequest `json:"merge_request"`
}

func (h *CancelMergeWhenPipelineSucceedsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[CancelMergeWhenPipelineSucceedsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequest, _, err := git.MergeRequests.CancelMergeWhenPipelineSucceeds(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &CancelMergeWhenPipelineSucceedsOutputs{MergeRequest: mergeRequest}, nil
}
