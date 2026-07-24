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

type DeleteMultipleNetworkPoliciesHandler struct{}

func NewDeleteMultipleNetworkPoliciesHandler() *DeleteMultipleNetworkPoliciesHandler {
	return &DeleteMultipleNetworkPoliciesHandler{}
}

type DeleteMultipleNetworkPoliciesInputs struct {
	*support.DeleteFields
	*support.ListFields
	Namespace string `json:"namespace,omitempty"`
}

type DeleteMultipleNetworkPoliciesOutputs struct{}

func (h *DeleteMultipleNetworkPoliciesHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteMultipleNetworkPoliciesInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	err = client.NetworkingV1().NetworkPolicies(inputs.Namespace).DeleteCollection(ctx, support.MetaDelete(inputs.DeleteFields), support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &DeleteMultipleNetworkPoliciesOutputs{}, nil
}
