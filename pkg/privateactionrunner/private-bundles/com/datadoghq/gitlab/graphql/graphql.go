package com_datadoghq_gitlab_graphql

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
)

type GraphqlHandler struct{}

func NewGraphqlHandler() *GraphqlHandler {
	return &GraphqlHandler{}
}

type GraphqlInputs struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type GraphqlOutputs struct {
	Result any `json:"result"`
}

func (h *GraphqlHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *runtimepb.Credential,
) (any, error) {
	inputs, err := types.ExtractInputs[GraphqlInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	// TODO: Migrate to the experimental GraphQL endpoint once the SDK is officially released: https://gitlab.com/gitlab-org/api/client-go/-/blob/main/graphql.go
	req, err := git.NewRequest(http.MethodPost, "", inputs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab http request: %w", err)
	}
	req.URL.Path = "/api/graphql"

	var output any
	_, err = git.Do(req, &output)
	if err != nil {
		return nil, err
	}

	return &GraphqlOutputs{Result: output}, nil
}
