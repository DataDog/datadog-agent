package com_datadoghq_kubernetes_customresources

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type UpdateCustomObjectHandler struct{}

func NewUpdateCustomObjectHandler() *UpdateCustomObjectHandler {
	return &UpdateCustomObjectHandler{}
}

type UpdateCustomObjectInputs struct {
	*support.UpdateFields
	Namespace string                 `json:"namespace"`
	Group     string                 `json:"group"`
	Version   string                 `json:"version"`
	Plural    string                 `json:"plural"`
	Body      map[string]interface{} `json:"body,omitempty"`
}

type UpdateCustomObjectOutputs = map[string]interface{}

func (h *UpdateCustomObjectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateCustomObjectInputs](task)
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

	resp, err := client.Resource(gvr).Namespace(inputs.Namespace).Update(ctx, &unstructured.Unstructured{Object: inputs.Body}, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return resp.Object, nil
}
