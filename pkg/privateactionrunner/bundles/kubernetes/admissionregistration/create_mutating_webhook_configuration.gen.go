// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_admissionregistration


import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreateMutatingWebhookConfigurationHandler struct{}

func NewCreateMutatingWebhookConfigurationHandler() *CreateMutatingWebhookConfigurationHandler {
	return &CreateMutatingWebhookConfigurationHandler{}
}

type CreateMutatingWebhookConfigurationInputs struct {
	*support.CreateFields
	Body *v1.MutatingWebhookConfiguration `json:"body,omitempty"`
}

type CreateMutatingWebhookConfigurationOutputs struct {
	ObjectMeta metav1.ObjectMeta    `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Webhooks   []v1.MutatingWebhook `json:"webhooks,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=Webhooks"`
}

func (h *CreateMutatingWebhookConfigurationHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateMutatingWebhookConfigurationInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(ctx, inputs.Body, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return &CreateMutatingWebhookConfigurationOutputs{
		ObjectMeta: response.ObjectMeta,
		Webhooks:   response.Webhooks,
	}, nil
}
