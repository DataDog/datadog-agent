// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_jobs

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabJobsBundle struct{}

func NewGitlabJobs() types.Bundle {
	return &GitlabJobsBundle{}
}

func (b *GitlabJobsBundle) GetAction(actionName string) types.Action {
	return b
}

func (b *GitlabJobsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "cancelJob":
		return b.RunCancelJob(ctx, task, credential)
	case "deleteArtifacts":
		return b.RunDeleteArtifacts(ctx, task, credential)
	case "deleteProjectArtifacts":
		return b.RunDeleteProjectArtifacts(ctx, task, credential)
	case "eraseJob":
		return b.RunEraseJob(ctx, task, credential)
	case "getJob":
		return b.RunGetJob(ctx, task, credential)
	case "getJobTokensJob":
		return b.RunGetJobTokensJob(ctx, task, credential)
	case "keepArtifacts":
		return b.RunKeepArtifacts(ctx, task, credential)
	case "listPipelineBridges":
		return b.RunListPipelineBridges(ctx, task, credential)
	case "listPipelineJobs":
		return b.RunListPipelineJobs(ctx, task, credential)
	case "listProjectJobs":
		return b.RunListProjectJobs(ctx, task, credential)
	case "playJob":
		return b.RunPlayJob(ctx, task, credential)
	case "retryJob":
		return b.RunRetryJob(ctx, task, credential)
	default:
		return nil, fmt.Errorf("action not found %s", actionName)
	}
}
