package com_datadoghq_gitlab_jobs

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabJobsBundle struct {
	actions map[string]types.Action
}

func NewGitlabJobs() types.Bundle {
	return &GitlabJobsBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"cancelJob":              NewCancelJobHandler(),
			"deleteArtifacts":        NewDeleteArtifactsHandler(),
			"deleteProjectArtifacts": NewDeleteProjectArtifactsHandler(),
			"eraseJob":               NewEraseJobHandler(),
			"getJob":                 NewGetJobHandler(),
			"getJobTokensJob":        NewGetJobTokensJobHandler(),
			"keepArtifacts":          NewKeepArtifactsHandler(),
			"listPipelineBridges":    NewListPipelineBridgesHandler(),
			"listPipelineJobs":       NewListPipelineJobsHandler(),
			"listProjectJobs":        NewListProjectJobsHandler(),
			"playJob":                NewPlayJobHandler(),
			"retryJob":               NewRetryJobHandler(),
		},
	}
}

func (h *GitlabJobsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
