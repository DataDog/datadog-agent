// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apiextensions

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesApiExtensions struct {
	actions map[string]types.Action
}

func NewKubernetesApiExtensions() *KubernetesApiExtensions {
	return &KubernetesApiExtensions{
		actions: map[string]types.Action{
			// Manual actions
			"createCustomResourceDefinition":          NewCreateCustomResourceDefinitionHandler(),
			"deleteCustomResourceDefinition":          NewDeleteCustomResourceDefinitionHandler(),
			"deleteMultipleCustomResourceDefinitions": NewDeleteMultipleCustomResourceDefinitionsHandler(),
			"getCustomResourceDefinition":             NewGetCustomResourceDefinitionHandler(),
			"listCustomResourceDefinition":            NewListCustomResourceDefinitionHandler(),
			"patchCustomResourceDefinition":           NewPatchCustomResourceDefinitionHandler(),
			"updateCustomResourceDefinition":          NewUpdateCustomResourceDefinitionHandler(),
		},
	}
}

func (h *KubernetesApiExtensions) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
