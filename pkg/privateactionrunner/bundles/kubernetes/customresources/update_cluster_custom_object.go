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

type UpdateClusterCustomObjectHandler struct{}

func NewUpdateClusterCustomObjectHandler() *UpdateClusterCustomObjectHandler {
	return &UpdateClusterCustomObjectHandler{}
}

type UpdateClusterCustomObjectInputs struct {
	*support.UpdateFields
	Group   string                 `json:"group"`
	Version string                 `json:"version"`
	Plural  string                 `json:"plural"`
	Body    map[string]interface{} `json:"body,omitempty"`
}

type UpdateClusterCustomObjectOutputs = map[string]interface{}

func (h *UpdateClusterCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateClusterCustomObjectInputs](task)
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

	resp, err := client.Resource(gvr).Update(ctx, &unstructured.Unstructured{Object: inputs.Body}, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
