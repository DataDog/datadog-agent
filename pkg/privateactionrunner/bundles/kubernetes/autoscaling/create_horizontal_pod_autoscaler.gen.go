// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_autoscaling

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreateHorizontalPodAutoscalerHandler struct{}

func NewCreateHorizontalPodAutoscalerHandler() *CreateHorizontalPodAutoscalerHandler {
	return &CreateHorizontalPodAutoscalerHandler{}
}

type CreateHorizontalPodAutoscalerInputs struct {
	*support.CreateFields
	Namespace string                      `json:"namespace,omitempty"`
	Body      *v1.HorizontalPodAutoscaler `json:"body,omitempty"`
}

type CreateHorizontalPodAutoscalerOutputs struct {
	ObjectMeta metav1.ObjectMeta                `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.HorizontalPodAutoscalerSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status     v1.HorizontalPodAutoscalerStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (h *CreateHorizontalPodAutoscalerHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateHorizontalPodAutoscalerInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.AutoscalingV1().HorizontalPodAutoscalers(inputs.Namespace).Create(ctx, inputs.Body, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return &CreateHorizontalPodAutoscalerOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
		Status:     response.Status,
	}, nil
}
