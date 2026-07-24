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

type CreateValidatingAdmissionPolicyBindingHandler struct{}

func NewCreateValidatingAdmissionPolicyBindingHandler() *CreateValidatingAdmissionPolicyBindingHandler {
	return &CreateValidatingAdmissionPolicyBindingHandler{}
}

type CreateValidatingAdmissionPolicyBindingInputs struct {
	*support.CreateFields
	Body *v1.ValidatingAdmissionPolicyBinding `json:"body,omitempty"`
}

type CreateValidatingAdmissionPolicyBindingOutputs struct {
	ObjectMeta metav1.ObjectMeta                       `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.ValidatingAdmissionPolicyBindingSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

func (h *CreateValidatingAdmissionPolicyBindingHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[CreateValidatingAdmissionPolicyBindingInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.AdmissionregistrationV1().ValidatingAdmissionPolicyBindings().Create(ctx, inputs.Body, support.MetaCreate(inputs.CreateFields))
	if err != nil {
		return nil, err
	}

	return &CreateValidatingAdmissionPolicyBindingOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
	}, nil
}
