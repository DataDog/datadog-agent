package com_datadoghq_gitlab_projects

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type RestoreProjectHandler struct{}

func NewRestoreProjectHandler() *RestoreProjectHandler {
	return &RestoreProjectHandler{}
}

type RestoreProjectInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
}

type RestoreProjectOutputs struct {
	Project *gitlab.Project `json:"project"`
}

func (h *RestoreProjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[RestoreProjectInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/restore", gitlab.PathEscape(inputs.ProjectId.String()))

	req, err := git.NewRequest(http.MethodPost, u, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	project := new(gitlab.Project)
	_, err = git.Do(req, &project)
	if err != nil {
		return nil, err
	}

	return &RestoreProjectOutputs{Project: project}, nil
}
