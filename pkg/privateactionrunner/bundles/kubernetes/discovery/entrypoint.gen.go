// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_discovery

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesDiscovery struct {
	actions map[string]types.Action
}

func NewKubernetesDiscovery() *KubernetesDiscovery {
	return &KubernetesDiscovery{
		actions: map[string]types.Action{
			// Manual actions
			// Auto-generated actions
			"createEndpointSlice":          NewCreateEndpointSliceHandler(),
			"updateEndpointSlice":          NewUpdateEndpointSliceHandler(),
			"deleteEndpointSlice":          NewDeleteEndpointSliceHandler(),
			"deleteMultipleEndpointSlices": NewDeleteMultipleEndpointSlicesHandler(),
			"getEndpointSlice":             NewGetEndpointSliceHandler(),
			"listEndpointSlice":            NewListEndpointSliceHandler(),
			"patchEndpointSlice":           NewPatchEndpointSliceHandler(),
		},
	}
}

func (h *KubernetesDiscovery) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
