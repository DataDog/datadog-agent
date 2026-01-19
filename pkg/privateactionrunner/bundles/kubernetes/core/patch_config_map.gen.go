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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
)

type PatchConfigMapHandler struct{}

func NewPatchConfigMapHandler() *PatchConfigMapHandler {
	return &PatchConfigMapHandler{}
}

type PatchConfigMapInputs struct {
	*support.PatchFields
	Namespace string `json:"namespace,omitempty"`
}

type PatchConfigMapOutputs struct {
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Immutable  *bool             `json:"immutable,omitempty" protobuf:"varint,4,opt,name=immutable"`
	Data       map[string]string `json:"data,omitempty" protobuf:"bytes,2,rep,name=data"`
	BinaryData map[string][]byte `json:"binaryData,omitempty" protobuf:"bytes,3,rep,name=binaryData"`
}

func (h *PatchConfigMapHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[PatchConfigMapInputs](task)
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

	response, err := client.CoreV1().ConfigMaps(inputs.Namespace).Patch(ctx, inputs.Name, typesv1.JSONPatchType, body, support.MetaPatch(inputs.PatchFields))
	if err != nil {
		return nil, err
	}

	return &PatchConfigMapOutputs{
		ObjectMeta: response.ObjectMeta,
		Immutable:  response.Immutable,
		Data:       response.Data,
		BinaryData: response.BinaryData,
	}, nil
}
