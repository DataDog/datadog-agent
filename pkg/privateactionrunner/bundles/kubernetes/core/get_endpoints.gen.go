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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GetEndpointsHandler struct{}

func NewGetEndpointsHandler() *GetEndpointsHandler {
	return &GetEndpointsHandler{}
}

type GetEndpointsInputs struct {
	*support.GetFields
	Namespace string `json:"namespace,omitempty"`
}

type GetEndpointsOutputs struct {
	ObjectMeta metav1.ObjectMeta   `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Subsets    []v1.EndpointSubset `json:"subsets,omitempty" protobuf:"bytes,2,rep,name=subsets"`
}

func (h *GetEndpointsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[GetEndpointsInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.CoreV1().Endpoints(inputs.Namespace).Get(ctx, inputs.Name, support.MetaGet(inputs.GetFields))
	if err != nil {
		return nil, err
	}

	return &GetEndpointsOutputs{
		ObjectMeta: response.ObjectMeta,
		Subsets:    response.Subsets,
	}, nil
}
