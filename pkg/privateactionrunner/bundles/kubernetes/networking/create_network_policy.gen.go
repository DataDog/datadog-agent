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
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreateNetworkPolicyHandler struct{}

func NewCreateNetworkPolicyHandler() *CreateNetworkPolicyHandler {
	return &CreateNetworkPolicyHandler{}
}

type CreateNetworkPolicyInputs struct {
	*support.CreateFields
	Namespace string            `json:"namespace,omitempty"`
	Body      *v1.NetworkPolicy `json:"body,omitempty"`
}

type CreateNetworkPolicyOutputs struct {
	ObjectMeta metav1.ObjectMeta    `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.NetworkPolicySpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

func (h *CreateNetworkPolicyHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateNetworkPolicyInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.NetworkingV1().NetworkPolicies(inputs.Namespace).Create(ctx, inputs.Body, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return &CreateNetworkPolicyOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
	}, nil
}
