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

type CreateClusterRoleHandler struct{}

func NewCreateClusterRoleHandler() *CreateClusterRoleHandler {
	return &CreateClusterRoleHandler{}
}

type CreateClusterRoleInputs struct {
	*support.CreateFields
	Body *v1.ClusterRole `json:"body,omitempty"`
}

type CreateClusterRoleOutputs struct {
	ObjectMeta      metav1.ObjectMeta   `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Rules           []v1.PolicyRule     `json:"rules" protobuf:"bytes,2,rep,name=rules"`
	AggregationRule *v1.AggregationRule `json:"aggregationRule,omitempty" protobuf:"bytes,3,opt,name=aggregationRule"`
}

func (h *CreateClusterRoleHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateClusterRoleInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.RbacV1().ClusterRoles().Create(ctx, inputs.Body, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return &CreateClusterRoleOutputs{
		ObjectMeta:      response.ObjectMeta,
		Rules:           response.Rules,
		AggregationRule: response.AggregationRule,
	}, nil
}
