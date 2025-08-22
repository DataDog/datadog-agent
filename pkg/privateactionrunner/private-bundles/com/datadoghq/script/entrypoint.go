// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
