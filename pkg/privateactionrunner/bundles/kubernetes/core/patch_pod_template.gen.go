// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
)

type PatchPodTemplateHandler struct{}

func NewPatchPodTemplateHandler() *PatchPodTemplateHandler {
	return &PatchPodTemplateHandler{}
}

type PatchPodTemplateInputs struct {
	*support.PatchFields
	Namespace string `json:"namespace,omitempty"`
}

type PatchPodTemplateOutputs struct {
	ObjectMeta metav1.ObjectMeta  `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Template   v1.PodTemplateSpec `json:"template,omitempty" protobuf:"bytes,2,opt,name=template"`
}

func (h *PatchPodTemplateHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[PatchPodTemplateInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(inputs.Body)
	if err != nil {
		return nil, err
	}

	response, err := client.CoreV1().PodTemplates(inputs.Namespace).Patch(ctx, inputs.Name, typesv1.JSONPatchType, body, support.MetaPatch(inputs.PatchFields))
	if err != nil {
		return nil, err
	}

	return &PatchPodTemplateOutputs{
		ObjectMeta: response.ObjectMeta,
		Template:   response.Template,
	}, nil
}
