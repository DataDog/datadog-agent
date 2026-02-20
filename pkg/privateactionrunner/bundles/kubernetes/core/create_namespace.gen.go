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

type CreateNamespaceHandler struct{}

func NewCreateNamespaceHandler() *CreateNamespaceHandler {
	return &CreateNamespaceHandler{}
}

type CreateNamespaceInputs struct {
	*support.CreateFields
	Body *v1.Namespace `json:"body,omitempty"`
}

type CreateNamespaceOutputs struct {
	ObjectMeta metav1.ObjectMeta  `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.NamespaceSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status     v1.NamespaceStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (h *CreateNamespaceHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateNamespaceInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.CoreV1().Namespaces().Create(ctx, inputs.Body, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return &CreateNamespaceOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
		Status:     response.Status,
	}, nil
}
