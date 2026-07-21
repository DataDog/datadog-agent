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

type UpdateValidatingWebhookConfigurationHandler struct{}

func NewUpdateValidatingWebhookConfigurationHandler() *UpdateValidatingWebhookConfigurationHandler {
	return &UpdateValidatingWebhookConfigurationHandler{}
}

type UpdateValidatingWebhookConfigurationInputs struct {
	*support.UpdateFields
	Body *v1.ValidatingWebhookConfiguration `json:"body,omitempty"`
}

type UpdateValidatingWebhookConfigurationOutputs struct {
	ObjectMeta metav1.ObjectMeta      `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Webhooks   []v1.ValidatingWebhook `json:"webhooks,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=Webhooks"`
}

func (h *UpdateValidatingWebhookConfigurationHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateValidatingWebhookConfigurationInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(ctx, inputs.Body, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return &UpdateValidatingWebhookConfigurationOutputs{
		ObjectMeta: response.ObjectMeta,
		Webhooks:   response.Webhooks,
	}, nil
}
