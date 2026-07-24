// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_admissionregistration


import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
)

type PatchValidatingAdmissionPolicyBindingHandler struct{}

func NewPatchValidatingAdmissionPolicyBindingHandler() *PatchValidatingAdmissionPolicyBindingHandler {
	return &PatchValidatingAdmissionPolicyBindingHandler{}
}

type PatchValidatingAdmissionPolicyBindingInputs struct {
	*support.PatchFields
}

type PatchValidatingAdmissionPolicyBindingOutputs struct {
	ObjectMeta metav1.ObjectMeta                       `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.ValidatingAdmissionPolicyBindingSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

func (h *PatchValidatingAdmissionPolicyBindingHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[PatchValidatingAdmissionPolicyBindingInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(inputs.Body)
	if err != nil {
		return nil, err
	}

	response, err := client.AdmissionregistrationV1().ValidatingAdmissionPolicyBindings().Patch(ctx, inputs.Name, typesv1.JSONPatchType, body, support.MetaPatch(inputs.PatchFields))
	if err != nil {
		return nil, err
	}

	return &PatchValidatingAdmissionPolicyBindingOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
	}, nil
}
