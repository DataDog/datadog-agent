// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_batch

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesBatch struct {
	actions map[string]types.Action
}

func NewKubernetesBatch() *KubernetesBatch {
	return &KubernetesBatch{
		actions: map[string]types.Action{
			// Manual actions
			// Auto-generated actions
			"createCronJob":          NewCreateCronJobHandler(),
			"updateCronJob":          NewUpdateCronJobHandler(),
			"deleteCronJob":          NewDeleteCronJobHandler(),
			"deleteMultipleCronJobs": NewDeleteMultipleCronJobsHandler(),
			"getCronJob":             NewGetCronJobHandler(),
			"listCronJob":            NewListCronJobHandler(),
			"patchCronJob":           NewPatchCronJobHandler(),
			"createJob":              NewCreateJobHandler(),
			"updateJob":              NewUpdateJobHandler(),
			"deleteJob":              NewDeleteJobHandler(),
			"deleteMultipleJobs":     NewDeleteMultipleJobsHandler(),
			"getJob":                 NewGetJobHandler(),
			"listJob":                NewListJobHandler(),
			"patchJob":               NewPatchJobHandler(),
		},
	}
}

func (h *KubernetesBatch) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
