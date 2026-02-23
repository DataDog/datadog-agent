// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_batch

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteCronJobHandler struct{}

func NewDeleteCronJobHandler() *DeleteCronJobHandler {
	return &DeleteCronJobHandler{}
}

type DeleteCronJobInputs struct {
	*support.DeleteFields
	Namespace string `json:"namespace,omitempty"`
}

type DeleteCronJobOutputs struct{}

func (h *DeleteCronJobHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteCronJobInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.BatchV1().CronJobs(inputs.Namespace).Delete(ctx, inputs.Name, support.MetaDelete(inputs.DeleteFields))
	if err != nil {
		return nil, err
	}

	return &DeleteCronJobOutputs{}, nil
}
