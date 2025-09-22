package com_datadoghq_gitlab_jobs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteArtifactsHandler struct{}

func NewDeleteArtifactsHandler() *DeleteArtifactsHandler {
	return &DeleteArtifactsHandler{}
}

type DeleteArtifactsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	JobId     int          `json:"job_id,omitempty"`
}

type DeleteArtifactsOutputs struct{}

func (h *DeleteArtifactsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[DeleteArtifactsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.Jobs.DeleteArtifacts(inputs.ProjectId.String(), inputs.JobId)
	if err != nil {
		return nil, err
	}
	return &DeleteArtifactsOutputs{}, nil
}
