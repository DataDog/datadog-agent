// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apiextensions

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type CreateCustomResourceDefinitionHandler struct{}

func NewCreateCustomResourceDefinitionHandler() *CreateCustomResourceDefinitionHandler {
	return &CreateCustomResourceDefinitionHandler{}
}

type CreateCustomResourceDefinitionInputs struct {
	*support.CreateFields
	Body map[string]interface{} `json:"body,omitempty"`
}

type CreateCustomResourceDefinitionOutputs = map[string]interface{}

func (h *CreateCustomResourceDefinitionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateCustomResourceDefinitionInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.DynamicKubeClient(credential)
	if err != nil {
		return nil, err
	}

	gvr := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	resp, err := client.Resource(gvr).Create(ctx, &unstructured.Unstructured{Object: inputs.Body}, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
