// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Jenkins struct {
	actions map[string]types.Action
}

func NewJenkins(runnerConfig *config.Config) *Jenkins {
	return &Jenkins{
		actions: map[string]types.Action{
			"buildJenkinsJob":  NewBuildJobHandler(runnerConfig),
			"getJobStatus":     NewGetJobStatusHandler(runnerConfig),
			"deleteJenkinsJob": NewDeleteJobHandler(runnerConfig),
		},
	}
}

func (h *Jenkins) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
