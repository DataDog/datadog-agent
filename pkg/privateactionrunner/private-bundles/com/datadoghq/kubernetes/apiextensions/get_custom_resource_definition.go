package com_datadoghq_kubernetes_apiextensions

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GetCustomResourceDefinitionHandler struct{}

func NewGetCustomResourceDefinitionHandler() *GetCustomResourceDefinitionHandler {
	return &GetCustomResourceDefinitionHandler{}
}

type GetCustomResourceDefinitionInputs struct {
	*support.GetFields
}

type GetCustomResourceDefinitionOutputs = map[string]interface{}

func (h *GetCustomResourceDefinitionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[GetCustomResourceDefinitionInputs](task)
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

	resp, err := client.Resource(gvr).Get(ctx, inputs.Name, support.MetaGet(inputs.GetFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
