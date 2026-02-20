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

type GetLimitRangeHandler struct{}

func NewGetLimitRangeHandler() *GetLimitRangeHandler {
	return &GetLimitRangeHandler{}
}

type GetLimitRangeInputs struct {
	*support.GetFields
	Namespace string `json:"namespace,omitempty"`
}

type GetLimitRangeOutputs struct {
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.LimitRangeSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

func (h *GetLimitRangeHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[GetLimitRangeInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.CoreV1().LimitRanges(inputs.Namespace).Get(ctx, inputs.Name, support.MetaGet(inputs.GetFields))
	if err != nil {
		return nil, err
	}

	return &GetLimitRangeOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
	}, nil
}
