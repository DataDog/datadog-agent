// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type UpdateControllerRevisionHandler struct{}

func NewUpdateControllerRevisionHandler() *UpdateControllerRevisionHandler {
	return &UpdateControllerRevisionHandler{}
}

type UpdateControllerRevisionInputs struct {
	*support.UpdateFields
	Namespace string                 `json:"namespace,omitempty"`
	Body      *v1.ControllerRevision `json:"body,omitempty"`
}

type UpdateControllerRevisionOutputs struct {
	ObjectMeta metav1.ObjectMeta    `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Data       runtime.RawExtension `json:"data,omitempty" protobuf:"bytes,2,opt,name=data"`
	Revision   int64                `json:"revision" protobuf:"varint,3,opt,name=revision"`
}

func (h *UpdateControllerRevisionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateControllerRevisionInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.AppsV1().ControllerRevisions(inputs.Namespace).Update(ctx, inputs.Body, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return &UpdateControllerRevisionOutputs{
		ObjectMeta: response.ObjectMeta,
		Data:       response.Data,
		Revision:   response.Revision,
	}, nil
}
