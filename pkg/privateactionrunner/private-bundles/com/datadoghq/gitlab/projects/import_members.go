package com_datadoghq_gitlab_projects

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ImportMembersHandler struct{}

func NewImportMembersHandler() *ImportMembersHandler {
	return &ImportMembersHandler{}
}

type ImportMembersInputs struct {
	ProjectId       lib.GitlabID `json:"project_id"`
	SourceProjectId lib.GitlabID `json:"source_project_id"`
}

type ImportMembersOutputs struct{}

func (h *ImportMembersHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[ImportMembersInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("projects/%s/import_project_members/%s", gitlab.PathEscape(inputs.ProjectId.String()), gitlab.PathEscape(inputs.SourceProjectId.String()))

	req, err := git.NewRequest(http.MethodPost, u, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	_, err = git.Do(req, nil)
	if err != nil {
		return nil, err
	}

	return &ImportMembersOutputs{}, nil
}
