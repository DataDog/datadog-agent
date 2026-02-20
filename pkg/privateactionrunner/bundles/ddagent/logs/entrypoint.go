// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// LogsBundle implements the types.Bundle interface for the com.datadoghq.ddagent.logs bundle.
type LogsBundle struct {
	actions map[string]types.Action
}

// NewLogs creates a new LogsBundle with all log-related actions registered.
func NewLogs(wmeta workloadmeta.Component) types.Bundle {
	return &LogsBundle{
		actions: map[string]types.Action{
			"headProcessLog": NewHeadProcessLogHandler(),
			"tailProcessLog": NewTailProcessLogHandler(),
			"listFiles":      NewListFilesHandler(wmeta),
		},
	}
}

// GetAction returns the action registered under the given name.
func (b *LogsBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}
