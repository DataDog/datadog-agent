// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_autoscaling

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesAutoscaling struct {
	actions map[string]types.Action
}

func NewKubernetesAutoscaling() *KubernetesAutoscaling {
	return &KubernetesAutoscaling{
		actions: map[string]types.Action{
			"updateAutoscalerLimits": NewUpdateAutoscalerLimitsHandler(),
		},
	}
}

func (h *KubernetesAutoscaling) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
