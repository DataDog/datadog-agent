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

type UpdateServiceAccountHandler struct{}

func NewUpdateServiceAccountHandler() *UpdateServiceAccountHandler {
	return &UpdateServiceAccountHandler{}
}

type UpdateServiceAccountInputs struct {
	*support.UpdateFields
	Namespace string             `json:"namespace,omitempty"`
	Body      *v1.ServiceAccount `json:"body,omitempty"`
}

type UpdateServiceAccountOutputs struct {
	ObjectMeta                   metav1.ObjectMeta         `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Secrets                      []v1.ObjectReference      `json:"secrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=secrets"`
	ImagePullSecrets             []v1.LocalObjectReference `json:"imagePullSecrets,omitempty" protobuf:"bytes,3,rep,name=imagePullSecrets"`
	AutomountServiceAccountToken *bool                     `json:"automountServiceAccountToken,omitempty" protobuf:"varint,4,opt,name=automountServiceAccountToken"`
}

func (h *UpdateServiceAccountHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateServiceAccountInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.CoreV1().ServiceAccounts(inputs.Namespace).Update(ctx, inputs.Body, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return &UpdateServiceAccountOutputs{
		ObjectMeta:                   response.ObjectMeta,
		Secrets:                      response.Secrets,
		ImagePullSecrets:             response.ImagePullSecrets,
		AutomountServiceAccountToken: response.AutomountServiceAccountToken,
	}, nil
}
