// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apiextensions

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime/schema"
	typesv1 "k8s.io/apimachinery/pkg/types"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PatchCustomResourceDefinitionHandler struct{}

func NewPatchCustomResourceDefinitionHandler() *PatchCustomResourceDefinitionHandler {
	return &PatchCustomResourceDefinitionHandler{}
}

type PatchCustomResourceDefinitionInputs struct {
	*support.PatchFields
}

type PatchCustomResourceDefinitionOutputs = map[string]interface{}

func (h *PatchCustomResourceDefinitionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[PatchCustomResourceDefinitionInputs](task)
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

	body, err := json.Marshal(inputs.Body)
	if err != nil {
		return nil, err
	}

	resp, err := client.Resource(gvr).Patch(ctx, inputs.Name, typesv1.JSONPatchType, body, support.MetaPatch(inputs.PatchFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
