// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type KubernetesApps struct {
}

func NewKubernetesApps() *KubernetesApps {
	return &KubernetesApps{}
}

func (b *KubernetesApps) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createControllerRevision":
		return b.RunCreateControllerRevision(ctx, task, credential)
	case "createDaemonSet":
		return b.RunCreateDaemonSet(ctx, task, credential)
	case "createDeployment":
		return b.RunCreateDeployment(ctx, task, credential)
	case "createReplicaSet":
		return b.RunCreateReplicaSet(ctx, task, credential)
	case "createStatefulSet":
		return b.RunCreateStatefulSet(ctx, task, credential)
	case "deleteControllerRevision":
		return b.RunDeleteControllerRevision(ctx, task, credential)
	case "deleteDaemonSet":
		return b.RunDeleteDaemonSet(ctx, task, credential)
	case "deleteDeployment":
		return b.RunDeleteDeployment(ctx, task, credential)
	case "deleteReplicaSet":
		return b.RunDeleteReplicaSet(ctx, task, credential)
	case "deleteStatefulSet":
		return b.RunDeleteStatefulSet(ctx, task, credential)
	case "deleteMultipleControllerRevisions":
		return b.RunDeleteMultipleControllerRevisions(ctx, task, credential)
	case "deleteMultipleDaemonSets":
		return b.RunDeleteMultipleDaemonSets(ctx, task, credential)
	case "deleteMultipleDeployments":
		return b.RunDeleteMultipleDeployments(ctx, task, credential)
	case "deleteMultipleReplicaSets":
		return b.RunDeleteMultipleReplicaSets(ctx, task, credential)
	case "deleteMultipleStatefulSets":
		return b.RunDeleteMultipleStatefulSets(ctx, task, credential)
	case "getControllerRevision":
		return b.RunGetControllerRevision(ctx, task, credential)
	case "getDaemonSet":
		return b.RunGetDaemonSet(ctx, task, credential)
	case "getDeployment":
		return b.RunGetDeployment(ctx, task, credential)
	case "getReplicaSet":
		return b.RunGetReplicaSet(ctx, task, credential)
	case "getStatefulSet":
		return b.RunGetStatefulSet(ctx, task, credential)
	case "listControllerRevision":
		return b.RunListControllerRevision(ctx, task, credential)
	case "listDaemonSet":
		return b.RunListDaemonSet(ctx, task, credential)
	case "listDeployment":
		return b.RunListDeployment(ctx, task, credential)
	case "listReplicaSet":
		return b.RunListReplicaSet(ctx, task, credential)
	case "listStatefulSet":
		return b.RunListStatefulSet(ctx, task, credential)
	case "patchControllerRevision":
		return b.RunPatchControllerRevision(ctx, task, credential)
	case "patchDaemonSet":
		return b.RunPatchDaemonSet(ctx, task, credential)
	case "patchDeployment":
		return b.RunPatchDeployment(ctx, task, credential)
	case "patchReplicaSet":
		return b.RunPatchReplicaSet(ctx, task, credential)
	case "patchStatefulSet":
		return b.RunPatchStatefulSet(ctx, task, credential)
	case "updateControllerRevision":
		return b.RunUpdateControllerRevision(ctx, task, credential)
	case "updateDaemonSet":
		return b.RunUpdateDaemonSet(ctx, task, credential)
	case "updateDeployment":
		return b.RunUpdateDeployment(ctx, task, credential)
	case "updateReplicaSet":
		return b.RunUpdateReplicaSet(ctx, task, credential)
	case "updateStatefulSet":
		return b.RunUpdateStatefulSet(ctx, task, credential)
	case "restartDeployment":
		return b.RunRestartDeployment(ctx, task, credential)
	case "rollbackDeployment":
		return b.RunRollbackDeployment(ctx, task, credential)
	case "scaleDeployment":
		return b.RunScaleDeployment(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *KubernetesApps) GetAction(_ string) types.Action {
	return b
}
