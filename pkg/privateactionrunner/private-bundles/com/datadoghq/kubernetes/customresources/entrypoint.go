package com_datadoghq_kubernetes_customresources

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesCustomResources struct {
	actions map[string]types.Action
}

func NewKubernetesCustomResources() *KubernetesCustomResources {
	return &KubernetesCustomResources{
		actions: map[string]types.Action{
			// Manual actions
			"createCustomObject":          NewCreateCustomObjectHandler(),
			"deleteCustomObject":          NewDeleteCustomObjectHandler(),
			"deleteMultipleCustomObjects": NewDeleteMultipleCustomObjectsHandler(),
			"getCustomObject":             NewGetCustomObjectHandler(),
			"listCustomObject":            NewListCustomObjectHandler(),
			"patchCustomObject":           NewPatchCustomObjectHandler(),
			"updateCustomObject":          NewUpdateCustomObjectHandler(),
		},
	}
}

func (h *KubernetesCustomResources) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
