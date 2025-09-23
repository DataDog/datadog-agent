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

type ListMergeRequestPipelinesHandler struct{}

func NewListMergeRequestPipelinesHandler() *ListMergeRequestPipelinesHandler {
	return &ListMergeRequestPipelinesHandler{}
}

type ListMergeRequestPipelinesInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	Page            int          `json:"page,omitempty"`
	PerPage         int          `json:"per_page,omitempty"`
}

type ListMergeRequestPipelinesOutputs struct {
	PipelineInfos []*gitlab.PipelineInfo `json:"pipeline_infos"`
}

func (h *ListMergeRequestPipelinesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListMergeRequestPipelinesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	pipelineInfos, _, err := git.MergeRequests.ListMergeRequestPipelines(inputs.ProjectId.String(), inputs.MergeRequestIid, lib.WithPagination(inputs.Page, inputs.PerPage))
	if err != nil {
		return nil, err
	}
	return &ListMergeRequestPipelinesOutputs{PipelineInfos: pipelineInfos}, nil
}
