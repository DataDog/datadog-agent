package com_datadoghq_script

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Script struct {
	actions map[string]types.Action
}

func NewScript() *Script {
	return &Script{
		actions: map[string]types.Action{
			"runPredefinedScript": NewRunPredefinedScriptHandler(),
		},
	}
}

func (h *Script) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
