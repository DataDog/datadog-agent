// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_discovery

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DeleteMultipleEndpointSlicesHandler struct{}

func NewDeleteMultipleEndpointSlicesHandler() *DeleteMultipleEndpointSlicesHandler {
	return &DeleteMultipleEndpointSlicesHandler{}
}

type DeleteMultipleEndpointSlicesInputs struct {
	*support.DeleteFields
	*support.ListFields
	Namespace string `json:"namespace,omitempty"`
}

type DeleteMultipleEndpointSlicesOutputs struct{}

func (h *DeleteMultipleEndpointSlicesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteMultipleEndpointSlicesInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.DiscoveryV1().EndpointSlices(inputs.Namespace).DeleteCollection(ctx, support.MetaDelete(inputs.DeleteFields), support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &DeleteMultipleEndpointSlicesOutputs{}, nil
}
