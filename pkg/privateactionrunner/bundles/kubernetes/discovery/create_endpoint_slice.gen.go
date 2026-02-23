// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_discovery

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreateEndpointSliceHandler struct{}

func NewCreateEndpointSliceHandler() *CreateEndpointSliceHandler {
	return &CreateEndpointSliceHandler{}
}

type CreateEndpointSliceInputs struct {
	*support.CreateFields
	Namespace string            `json:"namespace,omitempty"`
	Body      *v1.EndpointSlice `json:"body,omitempty"`
}

type CreateEndpointSliceOutputs struct {
	ObjectMeta  metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	AddressType v1.AddressType    `json:"addressType" protobuf:"bytes,4,rep,name=addressType"`
	Endpoints   []v1.Endpoint     `json:"endpoints" protobuf:"bytes,2,rep,name=endpoints"`
	Ports       []v1.EndpointPort `json:"ports" protobuf:"bytes,3,rep,name=ports"`
}

func (h *CreateEndpointSliceHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateEndpointSliceInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.DiscoveryV1().EndpointSlices(inputs.Namespace).Create(ctx, inputs.Body, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return &CreateEndpointSliceOutputs{
		ObjectMeta:  response.ObjectMeta,
		AddressType: response.AddressType,
		Endpoints:   response.Endpoints,
		Ports:       response.Ports,
	}, nil
}
