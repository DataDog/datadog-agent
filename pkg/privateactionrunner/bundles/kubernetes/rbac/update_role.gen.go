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

type UpdateRoleHandler struct{}

func NewUpdateRoleHandler() *UpdateRoleHandler {
	return &UpdateRoleHandler{}
}

type UpdateRoleInputs struct {
	*support.UpdateFields
	Namespace string   `json:"namespace,omitempty"`
	Body      *v1.Role `json:"body,omitempty"`
}

type UpdateRoleOutputs struct {
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Rules      []v1.PolicyRule   `json:"rules" protobuf:"bytes,2,rep,name=rules"`
}

func (h *UpdateRoleHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateRoleInputs](task)
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

	response, err := client.RbacV1().Roles(inputs.Namespace).Update(ctx, inputs.Body, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return &UpdateRoleOutputs{
		ObjectMeta: response.ObjectMeta,
		Rules:      response.Rules,
	}, nil
}
