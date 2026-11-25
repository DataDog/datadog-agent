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

type GetClusterRoleHandler struct{}

func NewGetClusterRoleHandler() *GetClusterRoleHandler {
	return &GetClusterRoleHandler{}
}

type GetClusterRoleInputs struct {
	*support.GetFields
}

type GetClusterRoleOutputs struct {
	ObjectMeta      metav1.ObjectMeta   `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Rules           []v1.PolicyRule     `json:"rules" protobuf:"bytes,2,rep,name=rules"`
	AggregationRule *v1.AggregationRule `json:"aggregationRule,omitempty" protobuf:"bytes,3,opt,name=aggregationRule"`
}

func (h *GetClusterRoleHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[GetClusterRoleInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.GetFields == nil || inputs.Name == "" {
		return nil, errors.New("name is required")
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.RbacV1().ClusterRoles().Get(ctx, inputs.Name, support.MetaGet(inputs.GetFields))
	if err != nil {
		return nil, err
	}

	return &GetClusterRoleOutputs{
		ObjectMeta:      response.ObjectMeta,
		Rules:           response.Rules,
		AggregationRule: response.AggregationRule,
	}, nil
}
