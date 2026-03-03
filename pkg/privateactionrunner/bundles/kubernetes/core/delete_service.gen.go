// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteServiceHandler struct{}

func NewDeleteServiceHandler() *DeleteServiceHandler {
	return &DeleteServiceHandler{}
}

type DeleteServiceInputs struct {
	*support.DeleteFields
	Namespace string `json:"namespace,omitempty"`
}

type DeleteServiceOutputs struct{}

func (h *DeleteServiceHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteServiceInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.CoreV1().Services(inputs.Namespace).Delete(ctx, inputs.Name, support.MetaDelete(inputs.DeleteFields))
	if err != nil {
		return nil, err
	}

	return &DeleteServiceOutputs{}, nil
}
