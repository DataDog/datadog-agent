// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_graphql

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GraphqlInputs struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type GraphqlOutputs struct {
	Result any `json:"result"`
}

func (b *GitlabGraphqlBundle) RunGraphql(
	ctx context.Context,
	task *types.Task, credential interface{},
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
