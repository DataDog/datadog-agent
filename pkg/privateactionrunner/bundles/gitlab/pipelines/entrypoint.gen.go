// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_pipelines

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabPipelinesBundle struct {
	actions map[string]types.Action
}

func NewGitlabPipelines() types.Bundle {
	return &GitlabPipelinesBundle{
		actions: map[string]types.Action{
			// Manual actions
			"createPipeline":       NewCreatePipelineHandler(),
			"listProjectPipelines": NewListProjectPipelinesHandler(),
			"retryPipelineBuild":   NewRetryPipelineBuildHandler(),
			// Auto-generated actions
			"cancelPipelineBuild":    NewCancelPipelineBuildHandler(),
			"deletePipeline":         NewDeletePipelineHandler(),
			"getLatestPipeline":      NewGetLatestPipelineHandler(),
			"getPipeline":            NewGetPipelineHandler(),
			"getPipelineTestReport":  NewGetPipelineTestReportHandler(),
			"getPipelineVariables":   NewGetPipelineVariablesHandler(),
			"updatePipelineMetadata": NewUpdatePipelineMetadataHandler(),
		},
	}
}

func (h *GitlabPipelinesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
