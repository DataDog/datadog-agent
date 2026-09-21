// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"encoding/json"

	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	support "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/kubernetes"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	"github.com/DataDog/dd-source/domains/actionplatform/libs/privateconnection"
)

type ScaleStatefulSetHorizontallyHandler struct {
	c kubernetes.Interface
}

func NewScaleStatefulSetHorizontallyHandler() *ScaleStatefulSetHorizontallyHandler {
	return &ScaleStatefulSetHorizontallyHandler{}
}

type ScaleStatefulSetHorizontallyInputs struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Replicas  int32  `json:"replicas,omitempty"`
}

type ScaleStatefulSetHorizontallyOutputs struct {
	ObjectMeta metav1.ObjectMeta    `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.StatefulSetSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status     v1.StatefulSetStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (h *ScaleStatefulSetHorizontallyHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[ScaleStatefulSetHorizontallyInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := h.getClient(credential)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": inputs.Replicas,
		},
	})
	if err != nil {
		return nil, err
	}

	response, err := client.AppsV1().StatefulSets(inputs.Namespace).Patch(ctx, inputs.Name, typesv1.MergePatchType, body, metav1.PatchOptions{})
	if err != nil {
		return nil, err
	}

	return &ScaleStatefulSetHorizontallyOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
		Status:     response.Status,
	}, nil
}

func (h *ScaleStatefulSetHorizontallyHandler) getClient(credential *privateconnection.PrivateCredentials) (kubernetes.Interface, error) {
	if h.c != nil {
		return h.c, nil
	}
	return support.KubeClient(credential)
}
