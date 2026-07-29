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

type ListNetworkPolicyHandler struct{}

func NewListNetworkPolicyHandler() *ListNetworkPolicyHandler {
	return &ListNetworkPolicyHandler{}
}

type ListNetworkPolicyInputs struct {
	*support.ListFields
	Namespace string `json:"namespace,omitempty"`
}

type ListNetworkPolicyOutputs struct {
	Items    []v1.NetworkPolicy `json:"items"`
	ListMeta metav1.ListMeta    `json:"metadata"`
}

func (h *ListNetworkPolicyHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[ListNetworkPolicyInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.NetworkingV1().NetworkPolicies(inputs.Namespace).List(ctx, support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &ListNetworkPolicyOutputs{
		Items:    response.Items,
		ListMeta: response.ListMeta,
	}, nil
}
