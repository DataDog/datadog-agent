package com_datadoghq_gitlab_notes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/lib"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PublishAllDraftNotesHandler struct{}

func NewPublishAllDraftNotesHandler() *PublishAllDraftNotesHandler {
	return &PublishAllDraftNotesHandler{}
}

type PublishAllDraftNotesInputs struct {
	ProjectId       lib.GitlabID `json:"project_id,omitempty"`
	MergeRequestIid int          `json:"merge_request_iid,omitempty"`
}

type PublishAllDraftNotesOutputs struct{}

func (h *PublishAllDraftNotesHandler) Run(
	ctx context.Context,
	task *types.Task, credential interface{},

) (any, error) {
	inputs, err := types.ExtractInputs[PublishAllDraftNotesInputs](task)
	if err != nil {
		return nil, err
	}
	git, err := lib.NewGitlabClient(credential)
	if err != nil {
		return nil, err
	}

	_, err = git.DraftNotes.PublishAllDraftNotes(inputs.ProjectId.String(), inputs.MergeRequestIid)
	if err != nil {
		return nil, err
	}
	return &PublishAllDraftNotesOutputs{}, nil
}
