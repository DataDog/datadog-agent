// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_rbac

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ListRoleBindingHandler struct{}

func NewListRoleBindingHandler() *ListRoleBindingHandler {
	return &ListRoleBindingHandler{}
}

type ListRoleBindingInputs struct {
	*support.ListFields
	Namespace string `json:"namespace,omitempty"`
}

type ListRoleBindingOutputs struct {
	Items    []v1.RoleBinding `json:"items"`
	ListMeta metav1.ListMeta  `json:"metadata"`
}

func (h *ListRoleBindingHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[ListRoleBindingInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.RbacV1().RoleBindings(inputs.Namespace).List(ctx, support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &ListRoleBindingOutputs{
		Items:    response.Items,
		ListMeta: response.ListMeta,
	}, nil
}
