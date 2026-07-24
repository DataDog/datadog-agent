// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_admissionregistration


import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesAdmissionregistration struct {
	actions map[string]types.Action
}

func NewKubernetesAdmissionregistration() *KubernetesAdmissionregistration {
	return &KubernetesAdmissionregistration{
		actions: map[string]types.Action{
			// Manual actions
			// Auto-generated actions
			"createMutatingWebhookConfiguration":              NewCreateMutatingWebhookConfigurationHandler(),
			"updateMutatingWebhookConfiguration":              NewUpdateMutatingWebhookConfigurationHandler(),
			"deleteMutatingWebhookConfiguration":              NewDeleteMutatingWebhookConfigurationHandler(),
			"deleteMultipleMutatingWebhookConfigurations":     NewDeleteMultipleMutatingWebhookConfigurationsHandler(),
			"getMutatingWebhookConfiguration":                 NewGetMutatingWebhookConfigurationHandler(),
			"listMutatingWebhookConfiguration":                NewListMutatingWebhookConfigurationHandler(),
			"patchMutatingWebhookConfiguration":               NewPatchMutatingWebhookConfigurationHandler(),
			"createValidatingAdmissionPolicy":                 NewCreateValidatingAdmissionPolicyHandler(),
			"updateValidatingAdmissionPolicy":                 NewUpdateValidatingAdmissionPolicyHandler(),
			"deleteValidatingAdmissionPolicy":                 NewDeleteValidatingAdmissionPolicyHandler(),
			"deleteMultipleValidatingAdmissionPolicies":       NewDeleteMultipleValidatingAdmissionPoliciesHandler(),
			"getValidatingAdmissionPolicy":                    NewGetValidatingAdmissionPolicyHandler(),
			"listValidatingAdmissionPolicy":                   NewListValidatingAdmissionPolicyHandler(),
			"patchValidatingAdmissionPolicy":                  NewPatchValidatingAdmissionPolicyHandler(),
			"createValidatingAdmissionPolicyBinding":          NewCreateValidatingAdmissionPolicyBindingHandler(),
			"updateValidatingAdmissionPolicyBinding":          NewUpdateValidatingAdmissionPolicyBindingHandler(),
			"deleteValidatingAdmissionPolicyBinding":          NewDeleteValidatingAdmissionPolicyBindingHandler(),
			"deleteMultipleValidatingAdmissionPolicyBindings": NewDeleteMultipleValidatingAdmissionPolicyBindingsHandler(),
			"getValidatingAdmissionPolicyBinding":             NewGetValidatingAdmissionPolicyBindingHandler(),
			"listValidatingAdmissionPolicyBinding":            NewListValidatingAdmissionPolicyBindingHandler(),
			"patchValidatingAdmissionPolicyBinding":           NewPatchValidatingAdmissionPolicyBindingHandler(),
			"createValidatingWebhookConfiguration":            NewCreateValidatingWebhookConfigurationHandler(),
			"updateValidatingWebhookConfiguration":            NewUpdateValidatingWebhookConfigurationHandler(),
			"deleteValidatingWebhookConfiguration":            NewDeleteValidatingWebhookConfigurationHandler(),
			"deleteMultipleValidatingWebhookConfigurations":   NewDeleteMultipleValidatingWebhookConfigurationsHandler(),
			"getValidatingWebhookConfiguration":               NewGetValidatingWebhookConfigurationHandler(),
			"listValidatingWebhookConfiguration":              NewListValidatingWebhookConfigurationHandler(),
			"patchValidatingWebhookConfiguration":             NewPatchValidatingWebhookConfigurationHandler(),
		},
	}
}

func (h *KubernetesAdmissionregistration) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
