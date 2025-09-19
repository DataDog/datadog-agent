package com_datadoghq_gitlab_labels

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabLabelsBundle struct {
	actions map[string]types.Action
}

func NewGitlabLabels() types.Bundle {
	return &GitlabLabelsBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"createLabel":          NewCreateLabelHandler(),
			"deleteLabel":          NewDeleteLabelHandler(),
			"getLabel":             NewGetLabelHandler(),
			"listLabels":           NewListLabelsHandler(),
			"promoteLabel":         NewPromoteLabelHandler(),
			"subscribeToLabel":     NewSubscribeToLabelHandler(),
			"unsubscribeFromLabel": NewUnsubscribeFromLabelHandler(),
			"updateLabel":          NewUpdateLabelHandler(),
		},
	}
}

func (h *GitlabLabelsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
