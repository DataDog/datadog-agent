// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Jenkins struct {
}

func NewJenkins() *Jenkins {
	return &Jenkins{}
}

func (j *Jenkins) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "buildJenkinsJob":
		return j.RunBuildJob(ctx, task, credential)
	case "getJobStatus":
		return j.RunGetJobStatus(ctx, task, credential)
	case "deleteJenkinsJob":
		return j.RunDeleteJob(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *Jenkins) GetAction(_ string) types.Action {
	return h
}
