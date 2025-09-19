package com_datadoghq_gitlab_deployments

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetProjectDeploymentHandler struct{}

func NewGetProjectDeploymentHandler() *GetProjectDeploymentHandler {
	return &GetProjectDeploymentHandler{}
}

type GetProjectDeploymentInputs struct {
	ProjectId    lib.GitlabID `json:"project_id,omitempty"`
	DeploymentId int          `json:"deployment_id,omitempty"`
}

type GetProjectDeploymentOutputs struct {
	Deployment *gitlab.Deployment `json:"deployment"`
}

func (h *GetProjectDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	deployment, _, err := git.Deployments.GetProjectDeployment(inputs.ProjectId.String(), inputs.DeploymentId)
	if err != nil {
		return nil, err
	}
	return &GetProjectDeploymentOutputs{Deployment: deployment}, nil
}
