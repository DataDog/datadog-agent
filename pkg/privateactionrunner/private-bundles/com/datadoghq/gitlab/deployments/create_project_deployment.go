package com_datadoghq_gitlab_deployments

import (
	"context"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type CreateProjectDeploymentHandler struct{}

func NewCreateProjectDeploymentHandler() *CreateProjectDeploymentHandler {
	return &CreateProjectDeploymentHandler{}
}

type CreateProjectDeploymentInputs struct {
	ProjectID lib.GitlabID `json:"project_id,omitempty"`
	*gitlab.CreateProjectDeploymentOptions
}

type CreateProjectDeploymentOutputs struct {
	Deployment *gitlab.Deployment `json:"deployment"`
}

func (h *CreateProjectDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[CreateProjectDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}
	deployment, _, err := git.Deployments.CreateProjectDeployment(inputs.ProjectID.String(), inputs.CreateProjectDeploymentOptions)
	if err != nil {
		return nil, err
	}
	return &CreateProjectDeploymentOutputs{Deployment: deployment}, nil
}
