// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_networking


import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteMultipleIngressClassesHandler struct{}

func NewDeleteMultipleIngressClassesHandler() *DeleteMultipleIngressClassesHandler {
	return &DeleteMultipleIngressClassesHandler{}
}

type DeleteMultipleIngressClassesInputs struct {
	*support.DeleteFields
	*support.ListFields
}

type DeleteMultipleIngressClassesOutputs struct{}

func (h *DeleteMultipleIngressClassesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteMultipleIngressClassesInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.NetworkingV1().IngressClasses().DeleteCollection(ctx, support.MetaDelete(inputs.DeleteFields), support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &DeleteMultipleIngressClassesOutputs{}, nil
}
