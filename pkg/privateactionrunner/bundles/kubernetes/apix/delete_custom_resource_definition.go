// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apix

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type DeleteCustomResourceDefinitionInputs struct {
	*support.DeleteFields
}

type DeleteCustomResourceDefinitionOutputs struct{}

func (b *KubernetesApiExtensions) RunDeleteCustomResourceDefinition(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (interface{}, error) {
	inputs, err := types.ExtractInputs[DeleteCustomResourceDefinitionInputs](task)
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

	err = client.Resource(gvr).Delete(ctx, inputs.Name, support.MetaDelete(inputs.DeleteFields))
	if err != nil {
		return nil, err
	}

	return &DeleteCustomResourceDefinitionOutputs{}, nil
}
