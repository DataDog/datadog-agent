// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesApps struct {
	actions map[string]types.Action
}

func NewKubernetesApps() *KubernetesApps {
	return &KubernetesApps{
		actions: map[string]types.Action{
			// Manual actions
			"restartDeployment":         NewRestartDeploymentHandler(),
			"rollbackDeployment":        NewRollbackDeploymentHandler(),
			"scaleDeployment":           NewScaleDeploymentHandler(),
			"scaleDeploymentVertically": NewScaleDeploymentVerticallyHandler(),
			// Auto-generated actions
			"createControllerRevision":          NewCreateControllerRevisionHandler(),
			"updateControllerRevision":          NewUpdateControllerRevisionHandler(),
			"deleteControllerRevision":          NewDeleteControllerRevisionHandler(),
			"deleteMultipleControllerRevisions": NewDeleteMultipleControllerRevisionsHandler(),
			"getControllerRevision":             NewGetControllerRevisionHandler(),
			"listControllerRevision":            NewListControllerRevisionHandler(),
			"patchControllerRevision":           NewPatchControllerRevisionHandler(),
			"createDaemonSet":                   NewCreateDaemonSetHandler(),
			"updateDaemonSet":                   NewUpdateDaemonSetHandler(),
			"deleteDaemonSet":                   NewDeleteDaemonSetHandler(),
			"deleteMultipleDaemonSets":          NewDeleteMultipleDaemonSetsHandler(),
			"getDaemonSet":                      NewGetDaemonSetHandler(),
			"listDaemonSet":                     NewListDaemonSetHandler(),
			"patchDaemonSet":                    NewPatchDaemonSetHandler(),
			"createDeployment":                  NewCreateDeploymentHandler(),
			"updateDeployment":                  NewUpdateDeploymentHandler(),
			"deleteDeployment":                  NewDeleteDeploymentHandler(),
			"deleteMultipleDeployments":         NewDeleteMultipleDeploymentsHandler(),
			"getDeployment":                     NewGetDeploymentHandler(),
			"listDeployment":                    NewListDeploymentHandler(),
			"patchDeployment":                   NewPatchDeploymentHandler(),
			"createReplicaSet":                  NewCreateReplicaSetHandler(),
			"updateReplicaSet":                  NewUpdateReplicaSetHandler(),
			"deleteReplicaSet":                  NewDeleteReplicaSetHandler(),
			"deleteMultipleReplicaSets":         NewDeleteMultipleReplicaSetsHandler(),
			"getReplicaSet":                     NewGetReplicaSetHandler(),
			"listReplicaSet":                    NewListReplicaSetHandler(),
			"patchReplicaSet":                   NewPatchReplicaSetHandler(),
			"createStatefulSet":                 NewCreateStatefulSetHandler(),
			"updateStatefulSet":                 NewUpdateStatefulSetHandler(),
			"deleteStatefulSet":                 NewDeleteStatefulSetHandler(),
			"deleteMultipleStatefulSets":        NewDeleteMultipleStatefulSetsHandler(),
			"getStatefulSet":                    NewGetStatefulSetHandler(),
			"listStatefulSet":                   NewListStatefulSetHandler(),
			"patchStatefulSet":                  NewPatchStatefulSetHandler(),
		},
	}
}

func (h *KubernetesApps) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
