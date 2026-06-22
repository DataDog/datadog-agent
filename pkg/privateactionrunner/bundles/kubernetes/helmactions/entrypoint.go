// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_helmactions

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesHelmActions struct {
	actions map[string]types.Action
}

func NewKubernetesHelmActions() *KubernetesHelmActions {
	return &KubernetesHelmActions{
		actions: map[string]types.Action{
			// Manual actions
			"rollback": NewRollbackHandler(),
		},
	}
}

func (h *KubernetesHelmActions) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
