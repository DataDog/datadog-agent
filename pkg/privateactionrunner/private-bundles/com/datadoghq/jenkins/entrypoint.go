package com_datadoghq_jenkins

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Jenkins struct {
	actions map[string]types.Action
}

func NewJenkins() *Jenkins {
	return &Jenkins{
		actions: map[string]types.Action{
			"buildJenkinsJob":  NewBuildJobHandler(),
			"getJobStatus":     NewGetJobStatusHandler(),
			"deleteJenkinsJob": NewDeleteJobHandler(),
		},
	}
}

func (h *Jenkins) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
