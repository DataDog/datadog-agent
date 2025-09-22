package com_datadoghq_kubernetes_apiextensions

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ListCustomResourceDefinitionHandler struct{}

func NewListCustomResourceDefinitionHandler() *ListCustomResourceDefinitionHandler {
	return &ListCustomResourceDefinitionHandler{}
}

type ListCustomResourceDefinitionInputs struct {
	*support.ListFields
}

type ListCustomResourceDefinitionOutputs struct {
	Items []unstructured.Unstructured `json:"items,omitempty"`
	Meta  interface{}                 `json:"metadata"`
}

func (h *ListCustomResourceDefinitionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[ListCustomResourceDefinitionInputs](task)
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

	resp, err := client.Resource(gvr).List(ctx, support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &ListCustomResourceDefinitionOutputs{
		Items: resp.Items,
		Meta:  resp.Object["metadata"],
	}, nil
}
