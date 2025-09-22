package com_datadoghq_kubernetes_apiextensions

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type UpdateCustomResourceDefinitionHandler struct{}

func NewUpdateCustomResourceDefinitionHandler() *UpdateCustomResourceDefinitionHandler {
	return &UpdateCustomResourceDefinitionHandler{}
}

type UpdateCustomResourceDefinitionInputs struct {
	*support.UpdateFields
	Body map[string]interface{} `json:"body,omitempty"`
}

type UpdateCustomResourceDefinitionOutputs = map[string]interface{}

func (h *UpdateCustomResourceDefinitionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateCustomResourceDefinitionInputs](task)
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

	resp, err := client.Resource(gvr).Update(ctx, &unstructured.Unstructured{Object: inputs.Body}, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
