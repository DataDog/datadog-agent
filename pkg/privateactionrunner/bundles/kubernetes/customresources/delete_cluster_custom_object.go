// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_customresources

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type DeleteClusterCustomObjectHandler struct{}

func NewDeleteClusterCustomObjectHandler() *DeleteClusterCustomObjectHandler {
	return &DeleteClusterCustomObjectHandler{}
}

type DeleteClusterCustomObjectInputs struct {
	*support.DeleteFields
	Group   string `json:"group"`
	Version string `json:"version"`
	Plural  string `json:"plural"`
}

type DeleteClusterCustomObjectOutputs struct{}

func (h *DeleteClusterCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteClusterCustomObjectInputs](task)
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

	err = client.Resource(gvr).Delete(ctx, inputs.Name, support.MetaDelete(inputs.DeleteFields))
	if err != nil {
		return nil, err
	}

	return &DeleteClusterCustomObjectOutputs{}, nil
}
