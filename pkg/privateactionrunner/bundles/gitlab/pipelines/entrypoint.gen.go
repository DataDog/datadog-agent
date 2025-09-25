// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_pipelines

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabPipelinesBundle struct{}

func NewGitlabPipelines() types.Bundle {
	return &GitlabPipelinesBundle{}
}

func (b *GitlabPipelinesBundle) GetAction(actionName string) types.Action {
	return b
}

func (b *GitlabPipelinesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createPipeline":
		return b.RunCreatePipeline(ctx, task, credential)
	case "listProjectPipelines":
		return b.RunListProjectPipelines(ctx, task, credential)
	case "retryPipelineBuild":
		return b.RunRetryPipelineBuild(ctx, task, credential)
	case "cancelPipelineBuild":
		return b.RunCancelPipelineBuild(ctx, task, credential)
	case "deletePipeline":
		return b.RunDeletePipeline(ctx, task, credential)
	case "getLatestPipeline":
		return b.RunGetLatestPipeline(ctx, task, credential)
	case "getPipeline":
		return b.RunGetPipeline(ctx, task, credential)
	case "getPipelineTestReport":
		return b.RunGetPipelineTestReport(ctx, task, credential)
	case "getPipelineVariables":
		return b.RunGetPipelineVariables(ctx, task, credential)
	case "updatePipelineMetadata":
		return b.RunUpdatePipelineMetadata(ctx, task, credential)
	default:
		return nil, fmt.Errorf("action not found %s", actionName)
	}
}
