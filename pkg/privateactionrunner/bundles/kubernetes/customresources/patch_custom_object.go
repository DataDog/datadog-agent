// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_customresources

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime/schema"
	typesv1 "k8s.io/apimachinery/pkg/types"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PatchCustomObjectHandler struct{}

func NewPatchCustomObjectHandler() *PatchCustomObjectHandler {
	return &PatchCustomObjectHandler{}
}

type PatchCustomObjectInputs struct {
	*support.PatchFields
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Plural    string `json:"plural"`
}

type PatchCustomObjectOutputs = map[string]interface{}

func (h *PatchCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[PatchCustomObjectInputs](task)
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

	body, err := json.Marshal(inputs.Body)
	if err != nil {
		return nil, err
	}

	resp, err := client.Resource(gvr).Namespace(inputs.Namespace).Patch(ctx, inputs.Name, typesv1.JSONPatchType, body, support.MetaPatch(inputs.PatchFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
