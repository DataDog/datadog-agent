// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
