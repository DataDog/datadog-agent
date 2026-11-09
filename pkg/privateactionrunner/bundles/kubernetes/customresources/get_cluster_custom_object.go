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
)

type GetClusterCustomObjectHandler struct{ policy resourcePolicy }

func NewGetClusterCustomObjectHandler(policy resourcePolicy) *GetClusterCustomObjectHandler {
	return &GetClusterCustomObjectHandler{policy: policy}
}

type GetClusterCustomObjectInputs struct {
	*support.GetFields
	Group   string `json:"group"`
	Version string `json:"version"`
	Plural  string `json:"plural"`
}

type GetClusterCustomObjectOutputs = map[string]interface{}

func (h *GetClusterCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[GetClusterCustomObjectInputs](task)
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

	resp, err := client.Resource(gvr).Get(ctx, inputs.Name, support.MetaGet(inputs.GetFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
