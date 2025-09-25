// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package com_datadoghq_kubernetes_batch

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type KubernetesBatch struct {
}

func NewKubernetesBatch() *KubernetesBatch {
	return &KubernetesBatch{}
}

func (b *KubernetesBatch) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createCronJob":
		return b.RunCreateCronJob(ctx, task, credential)
	case "createJob":
		return b.RunCreateJob(ctx, task, credential)
	case "deleteCronJob":
		return b.RunDeleteCronJob(ctx, task, credential)
	case "deleteJob":
		return b.RunDeleteJob(ctx, task, credential)
	case "deleteMultipleCronJobs":
		return b.RunDeleteMultipleCronJobs(ctx, task, credential)
	case "deleteMultipleJobs":
		return b.RunDeleteMultipleJobs(ctx, task, credential)
	case "getCronJob":
		return b.RunGetCronJob(ctx, task, credential)
	case "getJob":
		return b.RunGetJob(ctx, task, credential)
	case "listCronJob":
		return b.RunListCronJob(ctx, task, credential)
	case "listJob":
		return b.RunListJob(ctx, task, credential)
	case "patchCronJob":
		return b.RunPatchCronJob(ctx, task, credential)
	case "patchJob":
		return b.RunPatchJob(ctx, task, credential)
	case "updateCronJob":
		return b.RunUpdateCronJob(ctx, task, credential)
	case "updateJob":
		return b.RunUpdateJob(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *KubernetesBatch) GetAction(_ string) types.Action {
	return b
}
