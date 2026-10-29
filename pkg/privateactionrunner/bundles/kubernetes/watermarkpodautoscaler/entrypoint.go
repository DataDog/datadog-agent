// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_watermarkpodautoscaler

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesWatermarkPodAutoscaler struct {
	actions map[string]types.Action
}

func NewKubernetesWatermarkPodAutoscaler() *KubernetesWatermarkPodAutoscaler {
	return &KubernetesWatermarkPodAutoscaler{
		actions: map[string]types.Action{
			"updateWatermarkAutoscalerLimits": NewUpdateWatermarkAutoscalerLimitsHandler(),
		},
	}
}

func (h *KubernetesWatermarkPodAutoscaler) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
