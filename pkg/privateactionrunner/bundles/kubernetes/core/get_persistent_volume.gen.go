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

type GetPersistentVolumeHandler struct{}

func NewGetPersistentVolumeHandler() *GetPersistentVolumeHandler {
	return &GetPersistentVolumeHandler{}
}

type GetPersistentVolumeInputs struct {
	*support.GetFields
}

type GetPersistentVolumeOutputs struct {
	ObjectMeta metav1.ObjectMeta         `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.PersistentVolumeSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status     v1.PersistentVolumeStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (h *GetPersistentVolumeHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[GetPersistentVolumeInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.CoreV1().PersistentVolumes().Get(ctx, inputs.Name, support.MetaGet(inputs.GetFields))
	if err != nil {
		return nil, err
	}

	return &GetPersistentVolumeOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
		Status:     response.Status,
	}, nil
}
