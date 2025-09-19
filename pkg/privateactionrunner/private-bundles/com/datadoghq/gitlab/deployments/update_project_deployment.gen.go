package com_datadoghq_gitlab_deployments

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type UpdateProjectDeploymentHandler struct{}

func NewUpdateProjectDeploymentHandler() *UpdateProjectDeploymentHandler {
	return &UpdateProjectDeploymentHandler{}
}

type UpdateProjectDeploymentInputs struct {
	ProjectId    lib.GitlabID `json:"project_id,omitempty"`
	DeploymentId int          `json:"deployment_id,omitempty"`
	*gitlab.UpdateProjectDeploymentOptions
}

type UpdateProjectDeploymentOutputs struct {
	Deployment *gitlab.Deployment `json:"deployment"`
}

func (h *UpdateProjectDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[UpdateProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	deployment, _, err := git.Deployments.UpdateProjectDeployment(inputs.ProjectId.String(), inputs.DeploymentId, inputs.UpdateProjectDeploymentOptions)
	if err != nil {
		return nil, err
	}
	return &UpdateProjectDeploymentOutputs{Deployment: deployment}, nil
}
