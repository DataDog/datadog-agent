// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_autoscaling

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UpdateHorizontalPodAutoscalerHandler struct{}

func NewUpdateHorizontalPodAutoscalerHandler() *UpdateHorizontalPodAutoscalerHandler {
	return &UpdateHorizontalPodAutoscalerHandler{}
}

type UpdateHorizontalPodAutoscalerInputs struct {
	*support.UpdateFields
	Namespace string                      `json:"namespace,omitempty"`
	Body      *v1.HorizontalPodAutoscaler `json:"body,omitempty"`
}

type UpdateHorizontalPodAutoscalerOutputs struct {
	ObjectMeta metav1.ObjectMeta                `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.HorizontalPodAutoscalerSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status     v1.HorizontalPodAutoscalerStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (h *UpdateHorizontalPodAutoscalerHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateHorizontalPodAutoscalerInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.Namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if inputs.Body == nil {
		return nil, errors.New("body is required")
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.AutoscalingV2().HorizontalPodAutoscalers(inputs.Namespace).Update(ctx, inputs.Body, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return &UpdateHorizontalPodAutoscalerOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
		Status:     response.Status,
	}, nil
}
