// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_rbac

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesRbac struct {
	actions map[string]types.Action
}

func NewKubernetesRbac() *KubernetesRbac {
	return &KubernetesRbac{
		actions: map[string]types.Action{
			// Manual actions
			// Auto-generated actions
			"createClusterRole":                 NewCreateClusterRoleHandler(),
			"updateClusterRole":                 NewUpdateClusterRoleHandler(),
			"deleteClusterRole":                 NewDeleteClusterRoleHandler(),
			"deleteMultipleClusterRoles":        NewDeleteMultipleClusterRolesHandler(),
			"getClusterRole":                    NewGetClusterRoleHandler(),
			"listClusterRole":                   NewListClusterRoleHandler(),
			"patchClusterRole":                  NewPatchClusterRoleHandler(),
			"createClusterRoleBinding":          NewCreateClusterRoleBindingHandler(),
			"updateClusterRoleBinding":          NewUpdateClusterRoleBindingHandler(),
			"deleteClusterRoleBinding":          NewDeleteClusterRoleBindingHandler(),
			"deleteMultipleClusterRoleBindings": NewDeleteMultipleClusterRoleBindingsHandler(),
			"getClusterRoleBinding":             NewGetClusterRoleBindingHandler(),
			"listClusterRoleBinding":            NewListClusterRoleBindingHandler(),
			"patchClusterRoleBinding":           NewPatchClusterRoleBindingHandler(),
			"createRole":                        NewCreateRoleHandler(),
			"updateRole":                        NewUpdateRoleHandler(),
			"deleteRole":                        NewDeleteRoleHandler(),
			"deleteMultipleRoles":               NewDeleteMultipleRolesHandler(),
			"getRole":                           NewGetRoleHandler(),
			"listRole":                          NewListRoleHandler(),
			"patchRole":                         NewPatchRoleHandler(),
			"createRoleBinding":                 NewCreateRoleBindingHandler(),
			"updateRoleBinding":                 NewUpdateRoleBindingHandler(),
			"deleteRoleBinding":                 NewDeleteRoleBindingHandler(),
			"deleteMultipleRoleBindings":        NewDeleteMultipleRoleBindingsHandler(),
			"getRoleBinding":                    NewGetRoleBindingHandler(),
			"listRoleBinding":                   NewListRoleBindingHandler(),
			"patchRoleBinding":                  NewPatchRoleBindingHandler(),
		},
	}
}

func (h *KubernetesRbac) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
