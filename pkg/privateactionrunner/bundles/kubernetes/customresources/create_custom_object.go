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
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type CreateCustomObjectHandler struct{}

func NewCreateCustomObjectHandler() *CreateCustomObjectHandler {
	return &CreateCustomObjectHandler{}
}

type CreateCustomObjectInputs struct {
	*support.CreateFields
	Namespace string                 `json:"namespace"`
	Group     string                 `json:"group"`
	Version   string                 `json:"version"`
	Plural    string                 `json:"plural"`
	Body      map[string]interface{} `json:"body,omitempty"`
}

type CreateCustomObjectOutputs = map[string]interface{}

func (h *CreateCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateCustomObjectInputs](task)
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

	resp, err := client.Resource(gvr).Namespace(inputs.Namespace).Create(ctx, &unstructured.Unstructured{Object: inputs.Body}, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
