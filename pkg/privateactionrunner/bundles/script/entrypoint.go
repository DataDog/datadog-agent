// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
			"runPredefinedScript":           NewRunPredefinedScriptHandler(),
			"runPredefinedPowershellScript": NewRunPredefinedPowershellScriptHandler(),
			"runShellScript":                NewRunShellScriptHandler(),
			"testConnection":                NewTestConnectionHandler(),
			"enrichScript":                  NewEnrichScriptHandler(),
		},
	}
}

func (h *Script) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
