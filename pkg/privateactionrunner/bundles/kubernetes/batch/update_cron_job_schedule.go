// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_batch

import (
	"context"
	"encoding/json"
	"errors"

	batchv1 "k8s.io/api/batch/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UpdateCronJobScheduleHandler struct {
	client kubernetes.Interface
}

func NewUpdateCronJobScheduleHandler() *UpdateCronJobScheduleHandler {
	return &UpdateCronJobScheduleHandler{}
}

type UpdateCronJobScheduleInputs struct {
	Namespace string  `json:"namespace,omitempty"`
	Name      string  `json:"name,omitempty"`
	Schedule  *string `json:"schedule,omitempty"`
	TimeZone  *string `json:"timeZone,omitempty"`
	Suspend   *bool   `json:"suspend,omitempty"`
	DryRun    string  `json:"dryRun,omitempty"`
}

type UpdateCronJobScheduleOutputs struct {
	CronJob *batchv1.CronJob `json:"cronJob"`
}

func (h *UpdateCronJobScheduleHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateCronJobScheduleInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.Schedule == nil && inputs.TimeZone == nil && inputs.Suspend == nil {
		return nil, errors.New("at least one of schedule, timeZone, or suspend must be provided")
	}

	client, err := h.getClient(credential)
	if err != nil {
		return nil, err
	}

	specPatch := map[string]interface{}{}
	if inputs.Schedule != nil {
		specPatch["schedule"] = *inputs.Schedule
	}
	if inputs.TimeZone != nil {
		specPatch["timeZone"] = *inputs.TimeZone
	}
	if inputs.Suspend != nil {
		specPatch["suspend"] = *inputs.Suspend
	}

	body, err := json.Marshal(map[string]interface{}{
		"spec": specPatch,
	})
	if err != nil {
		return nil, err
	}

	response, err := client.BatchV1().CronJobs(inputs.Namespace).Patch(
		ctx,
		inputs.Name,
		typesv1.MergePatchType,
		body,
		support.MetaPatch(&support.PatchFields{DryRun: inputs.DryRun}),
	)
	if err != nil {
		return nil, err
	}

	return &UpdateCronJobScheduleOutputs{CronJob: response}, nil
}

func (h *UpdateCronJobScheduleHandler) getClient(credential *privateconnection.PrivateCredentials) (kubernetes.Interface, error) {
	if h.client != nil {
		return h.client, nil
	}
	return support.KubeClient(credential)
}
