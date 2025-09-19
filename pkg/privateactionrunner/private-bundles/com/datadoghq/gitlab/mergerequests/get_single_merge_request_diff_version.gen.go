package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetSingleMergeRequestDiffVersionHandler struct{}

func NewGetSingleMergeRequestDiffVersionHandler() *GetSingleMergeRequestDiffVersionHandler {
	return &GetSingleMergeRequestDiffVersionHandler{}
}

type GetSingleMergeRequestDiffVersionInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
	VersionId       int          `json:"version_id,omitempty"`
	*gitlab.GetSingleMergeRequestDiffVersionOptions
}

type GetSingleMergeRequestDiffVersionOutputs struct {
	MergeRequestDiffVersion *gitlab.MergeRequestDiffVersion `json:"merge_request_diff_version"`
}

func (h *GetSingleMergeRequestDiffVersionHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetSingleMergeRequestDiffVersionInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	mergeRequestDiffVersion, _, err := git.MergeRequests.GetSingleMergeRequestDiffVersion(inputs.ProjectId.String(), inputs.MergeRequestIid, inputs.VersionId, inputs.GetSingleMergeRequestDiffVersionOptions)
	if err != nil {
		return nil, err
	}
	return &GetSingleMergeRequestDiffVersionOutputs{MergeRequestDiffVersion: mergeRequestDiffVersion}, nil
}
