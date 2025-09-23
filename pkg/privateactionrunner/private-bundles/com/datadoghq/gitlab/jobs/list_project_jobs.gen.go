// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type ListProjectJobsHandler struct{}

func NewListProjectJobsHandler() *ListProjectJobsHandler {
	return &ListProjectJobsHandler{}
}

type ListProjectJobsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.ListJobsOptions
}

type ListProjectJobsOutputs struct {
	Jobs []*gitlab.Job `json:"jobs"`
}

func (h *ListProjectJobsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ListProjectJobsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	jobs, _, err := git.Jobs.ListProjectJobs(inputs.ProjectId.String(), inputs.ListJobsOptions)
	if err != nil {
		return nil, err
	}
	return &ListProjectJobsOutputs{Jobs: jobs}, nil
}
