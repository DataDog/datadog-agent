package com_datadoghq_temporal

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Temporal struct {
	actions map[string]types.Action
}

func NewTemporal() *Temporal {
	return &Temporal{
		actions: map[string]types.Action{
			"runWorkflow":       NewRunWorkflowHandler(),
			"listWorkflows":     NewListWorkflowsHandler(),
			"getWorkflowResult": NewGetWorkflowResultHandler(),
		},
	}
}

func (h *Temporal) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
