// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package com_datadoghq_kubernetes_core provides Kubernetes core functionality for private action bundles.
package com_datadoghq_kubernetes_core //nolint:revive

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

// KubernetesCore provides Kubernetes-related actions for private action bundles.
type KubernetesCore struct {
	actions map[string]types.Action
}

// NewKubernetesCore creates a new KubernetesCore instance.
func NewKubernetesCore() *KubernetesCore {
	return &KubernetesCore{
		actions: map[string]types.Action{
			// Manual actions
			// Auto-generated actions
			"listPod": NewListPodHandler(),
		},
	}
}

// GetAction returns the action with the specified name.
func (h *KubernetesCore) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
