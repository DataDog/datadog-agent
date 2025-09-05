package com_datadoghq_kubernetes_core

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesCore struct {
	actions map[string]types.Action
}

func NewKubernetesCore() *KubernetesCore {
	return &KubernetesCore{
		actions: map[string]types.Action{
			// Manual actions
			// Auto-generated actions
			"listPod": NewListPodHandler(),
		},
	}
}

func (h *KubernetesCore) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
