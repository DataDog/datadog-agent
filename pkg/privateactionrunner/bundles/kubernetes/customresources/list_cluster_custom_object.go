// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_customresources

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type ListClusterCustomObjectHandler struct{}

func NewListClusterCustomObjectHandler() *ListClusterCustomObjectHandler {
	return &ListClusterCustomObjectHandler{}
}

type ListClusterCustomObjectInputs struct {
	*support.ListFields
	Group   string `json:"group"`
	Version string `json:"version"`
	Plural  string `json:"plural"`
}

type ListClusterCustomObjectOutputs struct {
	Items []unstructured.Unstructured `json:"items,omitempty"`
	Meta  interface{}                 `json:"metadata"`
}

func (h *ListClusterCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[ListClusterCustomObjectInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.DynamicKubeClient(credential)
	if err != nil {
		return nil, err
	}

	gvr := schema.GroupVersionResource{
		Group:    inputs.Group,
		Version:  inputs.Version,
		Resource: inputs.Plural,
	}

	resp, err := client.Resource(gvr).List(ctx, support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &ListClusterCustomObjectOutputs{
		Items: resp.Items,
		Meta:  resp.Object["metadata"],
	}, nil
}
