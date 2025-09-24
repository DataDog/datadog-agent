// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetMergeRequestDiffVersionsHandler struct{}

func NewGetMergeRequestDiffVersionsHandler() *GetMergeRequestDiffVersionsHandler {
	return &GetMergeRequestDiffVersionsHandler{}
}

type GetMergeRequestDiffVersionsInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	*gitlab.GetMergeRequestDiffVersionsOptions
}

type GetMergeRequestDiffVersionsOutputs struct {
	MergeRequestDiffVersions []*gitlab.MergeRequestDiffVersion `json:"merge_request_diff_versions"`
}

func (h *GetMergeRequestDiffVersionsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetMergeRequestDiffVersionsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestDiffVersions, _, err := git.MergeRequests.GetMergeRequestDiffVersions(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.GetMergeRequestDiffVersionsOptions)
	if err != nil {
		return nil, err
	}
	return &GetMergeRequestDiffVersionsOutputs{MergeRequestDiffVersions: mergeRequestDiffVersions}, nil
}
