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

type DeleteMultiplePersistentVolumesHandler struct{}

func NewDeleteMultiplePersistentVolumesHandler() *DeleteMultiplePersistentVolumesHandler {
	return &DeleteMultiplePersistentVolumesHandler{}
}

type DeleteMultiplePersistentVolumesInputs struct {
	*support.DeleteFields
	*support.ListFields
}

type DeleteMultiplePersistentVolumesOutputs struct{}

func (h *DeleteMultiplePersistentVolumesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteMultiplePersistentVolumesInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.CoreV1().PersistentVolumes().DeleteCollection(ctx, support.MetaDelete(inputs.DeleteFields), support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &DeleteMultiplePersistentVolumesOutputs{}, nil
}
