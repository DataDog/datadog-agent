// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
