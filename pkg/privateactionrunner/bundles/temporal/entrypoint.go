// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Temporal struct {
}

func NewTemporal() *Temporal {
	return &Temporal{}
}

func (t *Temporal) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "runWorkflow":
		return t.RunWorkflow(ctx, task, credential)
	case "listWorkflows":
		return t.RunListWorkflows(ctx, task, credential)
	case "getWorkflowResult":
		return t.RunGetWorkflowResult(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (t *Temporal) GetAction(actionName string) types.Action {
	return t
}
