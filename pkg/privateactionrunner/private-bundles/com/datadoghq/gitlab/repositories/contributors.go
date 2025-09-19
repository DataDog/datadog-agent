package com_datadoghq_gitlab_repositories

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type ContributorsHandler struct{}

func NewContributorsHandler() *ContributorsHandler {
	return &ContributorsHandler{}
}

type ContributorsInputs struct {
	ProjectId lib.GitlabID `json:"project_id,omitempty"`
	*ListContributorsOptions
}

type ListContributorsOptions struct {
	Ref *string `url:"ref, omitempty" json:"ref,omitempty"`
	*gitlab.ListContributorsOptions
}

type ContributorsOutputs struct {
	Contributors []*gitlab.Contributor `json:"contributors"`
}

func (h *ContributorsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[ContributorsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/repository/contributors", gitlab.PathEscape(inputs.ProjectId.String()))

	req, err := git.NewRequest(http.MethodGet, u, inputs.ListContributorsOptions, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	var contributors []*gitlab.Contributor
	_, err = git.Do(req, &contributors)
	if err != nil {
		return nil, err
	}

	return &ContributorsOutputs{Contributors: contributors}, nil
}
