// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_rbac

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ListRoleHandler struct{}

func NewListRoleHandler() *ListRoleHandler {
	return &ListRoleHandler{}
}

type ListRoleInputs struct {
	*support.ListFields
	Namespace string `json:"namespace,omitempty"`
}

type ListRoleOutputs struct {
	Items    []v1.Role       `json:"items"`
	ListMeta metav1.ListMeta `json:"metadata"`
}

func (h *ListRoleHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[ListRoleInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.Namespace == "" {
		return nil, errors.New("namespace is required")
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.RbacV1().Roles(inputs.Namespace).List(ctx, support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &ListRoleOutputs{
		Items:    response.Items,
		ListMeta: response.ListMeta,
	}, nil
}
