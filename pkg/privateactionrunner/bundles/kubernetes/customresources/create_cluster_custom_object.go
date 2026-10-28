// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_customresources

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CreateClusterCustomObjectHandler struct{ policy resourcePolicy }

func NewCreateClusterCustomObjectHandler(policy resourcePolicy) *CreateClusterCustomObjectHandler {
	return &CreateClusterCustomObjectHandler{policy: policy}
}

type CreateClusterCustomObjectInputs struct {
	*support.CreateFields
	Group   string                 `json:"group"`
	Version string                 `json:"version"`
	Plural  string                 `json:"plural"`
	Body    map[string]interface{} `json:"body,omitempty"`
}

type CreateClusterCustomObjectOutputs = map[string]interface{}

func (h *CreateClusterCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateClusterCustomObjectInputs](task)
	if err != nil {
		return nil, err
	}

	gvr, err := h.policy.groupVersionResource(inputs.Group, inputs.Version, inputs.Plural)
	if err != nil {
		return nil, err
	}

	client, err := support.DynamicKubeClient(credential)
	if err != nil {
		return nil, err
	}

	resp, err := client.Resource(gvr).Create(ctx, &unstructured.Unstructured{Object: inputs.Body}, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
