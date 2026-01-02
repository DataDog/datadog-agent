// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/gitlab"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GetMergeRequestReviewersHandler struct{}

func NewGetMergeRequestReviewersHandler() *GetMergeRequestReviewersHandler {
	return &GetMergeRequestReviewersHandler{}
}

type GetMergeRequestReviewersInputs struct {
	ProjectId       support.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int64            `json:"merge_request_iid,omitempty"`
}

type GetMergeRequestReviewersOutputs struct {
	MergeRequestReviewers []*gitlab.MergeRequestReviewer `json:"merge_request_reviewers"`
}

func (h *GetMergeRequestReviewersHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[GetMergeRequestReviewersInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := support.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestReviewers, _, err := git.MergeRequests.GetMergeRequestReviewers(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &GetMergeRequestReviewersOutputs{MergeRequestReviewers: mergeRequestReviewers}, nil
}
