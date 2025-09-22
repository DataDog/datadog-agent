package com_datadoghq_kubernetes_customresources

import (
	"context"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type DeleteMultipleCustomObjectsHandler struct{}

func NewDeleteMultipleCustomObjectsHandler() *DeleteMultipleCustomObjectsHandler {
	return &DeleteMultipleCustomObjectsHandler{}
}

type DeleteMultipleCustomObjectsInputs struct {
	*support.DeleteFields
	*support.ListFields
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Plural    string `json:"plural"`
}

type DeleteMultipleCustomObjectsOutputs struct{}

func (h *DeleteMultipleCustomObjectsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[DeleteMultipleCustomObjectsInputs](task)
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

	err = client.Resource(gvr).Namespace(inputs.Namespace).DeleteCollection(ctx, support.MetaDelete(inputs.DeleteFields), support.MetaList(inputs.ListFields))
	if err != nil {
		return nil, err
	}

	return &DeleteMultipleCustomObjectsOutputs{}, nil
}
