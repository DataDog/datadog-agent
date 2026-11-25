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

type UpdateRoleBindingHandler struct{}

func NewUpdateRoleBindingHandler() *UpdateRoleBindingHandler {
	return &UpdateRoleBindingHandler{}
}

type UpdateRoleBindingInputs struct {
	*support.UpdateFields
	Namespace string          `json:"namespace,omitempty"`
	Body      *v1.RoleBinding `json:"body,omitempty"`
}

type UpdateRoleBindingOutputs struct {
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Subjects   []v1.Subject      `json:"subjects,omitempty" protobuf:"bytes,2,rep,name=subjects"`
	RoleRef    v1.RoleRef        `json:"roleRef" protobuf:"bytes,3,opt,name=roleRef"`
}

func (h *UpdateRoleBindingHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateRoleBindingInputs](task)
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

	response, err := client.RbacV1().RoleBindings(inputs.Namespace).Update(ctx, inputs.Body, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return &UpdateRoleBindingOutputs{
		ObjectMeta: response.ObjectMeta,
		Subjects:   response.Subjects,
		RoleRef:    response.RoleRef,
	}, nil
}
