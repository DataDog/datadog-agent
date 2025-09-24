// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_merge_requests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"gitlab.com/gitlab-org/api/client-go"
)

type GetMergeRequestParticipantsHandler struct{}

func NewGetMergeRequestParticipantsHandler() *GetMergeRequestParticipantsHandler {
	return &GetMergeRequestParticipantsHandler{}
}

type GetMergeRequestParticipantsInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
}

type GetMergeRequestParticipantsOutputs struct {
	BasicUsers []*gitlab.BasicUser `json:"basic_users"`
}

func (h *GetMergeRequestParticipantsHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[GetMergeRequestParticipantsInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	basicUsers, _, err := git.MergeRequests.GetMergeRequestParticipants(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &GetMergeRequestParticipantsOutputs{BasicUsers: basicUsers}, nil
}
