// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type KubernetesCore struct {
}

func NewKubernetesCore() *KubernetesCore {
	return &KubernetesCore{}
}

func (b *KubernetesCore) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createConfigMap":
		return b.RunCreateConfigMap(ctx, task, credential)
	case "createEndpoints":
		return b.RunCreateEndpoints(ctx, task, credential)
	case "createEvent":
		return b.RunCreateEvent(ctx, task, credential)
	case "createLimitRange":
		return b.RunCreateLimitRange(ctx, task, credential)
	case "createNamespace":
		return b.RunCreateNamespace(ctx, task, credential)
	case "createNode":
		return b.RunCreateNode(ctx, task, credential)
	case "createPersistentVolume":
		return b.RunCreatePersistentVolume(ctx, task, credential)
	case "createPersistentVolumeClaim":
		return b.RunCreatePersistentVolumeClaim(ctx, task, credential)
	case "createPod":
		return b.RunCreatePod(ctx, task, credential)
	case "createPodTemplate":
		return b.RunCreatePodTemplate(ctx, task, credential)
	case "createReplicationController":
		return b.RunCreateReplicationController(ctx, task, credential)
	case "createResourceQuota":
		return b.RunCreateResourceQuota(ctx, task, credential)
	case "createService":
		return b.RunCreateService(ctx, task, credential)
	case "createServiceAccount":
		return b.RunCreateServiceAccount(ctx, task, credential)
	case "deleteConfigMap":
		return b.RunDeleteConfigMap(ctx, task, credential)
	case "deleteEndpoints":
		return b.RunDeleteEndpoints(ctx, task, credential)
	case "deleteEvent":
		return b.RunDeleteEvent(ctx, task, credential)
	case "deleteLimitRange":
		return b.RunDeleteLimitRange(ctx, task, credential)
	case "deleteMultipleConfigMaps":
		return b.RunDeleteMultipleConfigMaps(ctx, task, credential)
	case "deleteMultipleEndpoints":
		return b.RunDeleteMultipleEndpoints(ctx, task, credential)
	case "deleteMultipleEvents":
		return b.RunDeleteMultipleEvents(ctx, task, credential)
	case "deleteMultipleLimitRanges":
		return b.RunDeleteMultipleLimitRanges(ctx, task, credential)
	case "deleteMultipleNodes":
		return b.RunDeleteMultipleNodes(ctx, task, credential)
	case "deleteMultiplePersistentVolumeClaims":
		return b.RunDeleteMultiplePersistentVolumeClaims(ctx, task, credential)
	case "deleteMultiplePersistentVolumes":
		return b.RunDeleteMultiplePersistentVolumes(ctx, task, credential)
	case "deleteMultiplePodTemplates":
		return b.RunDeleteMultiplePodTemplates(ctx, task, credential)
	case "deleteMultiplePods":
		return b.RunDeleteMultiplePods(ctx, task, credential)
	case "deleteMultipleReplicationControllers":
		return b.RunDeleteMultipleReplicationControllers(ctx, task, credential)
	case "deleteMultipleResourceQuotas":
		return b.RunDeleteMultipleResourceQuotas(ctx, task, credential)
	case "deleteMultipleServiceAccounts":
		return b.RunDeleteMultipleServiceAccounts(ctx, task, credential)
	case "deleteNamespace":
		return b.RunDeleteNamespace(ctx, task, credential)
	case "deleteNode":
		return b.RunDeleteNode(ctx, task, credential)
	case "deletePersistentVolume":
		return b.RunDeletePersistentVolume(ctx, task, credential)
	case "deletePersistentVolumeClaim":
		return b.RunDeletePersistentVolumeClaim(ctx, task, credential)
	case "deletePod":
		return b.RunDeletePod(ctx, task, credential)
	case "deletePodTemplate":
		return b.RunDeletePodTemplate(ctx, task, credential)
	case "deleteReplicationController":
		return b.RunDeleteReplicationController(ctx, task, credential)
	case "deleteResourceQuota":
		return b.RunDeleteResourceQuota(ctx, task, credential)
	case "deleteService":
		return b.RunDeleteService(ctx, task, credential)
	case "deleteServiceAccount":
		return b.RunDeleteServiceAccount(ctx, task, credential)
	case "getConfigMap":
		return b.RunGetConfigMap(ctx, task, credential)
	case "getEndpoints":
		return b.RunGetEndpoints(ctx, task, credential)
	case "getEvent":
		return b.RunGetEvent(ctx, task, credential)
	case "getLimitRange":
		return b.RunGetLimitRange(ctx, task, credential)
	case "getNamespace":
		return b.RunGetNamespace(ctx, task, credential)
	case "getNode":
		return b.RunGetNode(ctx, task, credential)
	case "getPersistentVolume":
		return b.RunGetPersistentVolume(ctx, task, credential)
	case "getPersistentVolumeClaim":
		return b.RunGetPersistentVolumeClaim(ctx, task, credential)
	case "getPod":
		return b.RunGetPod(ctx, task, credential)
	case "getPodTemplate":
		return b.RunGetPodTemplate(ctx, task, credential)
	case "getReplicationController":
		return b.RunGetReplicationController(ctx, task, credential)
	case "getResourceQuota":
		return b.RunGetResourceQuota(ctx, task, credential)
	case "getService":
		return b.RunGetService(ctx, task, credential)
	case "getServiceAccount":
		return b.RunGetServiceAccount(ctx, task, credential)
	case "listConfigMap":
		return b.RunListConfigMap(ctx, task, credential)
	case "listEndpoints":
		return b.RunListEndpoints(ctx, task, credential)
	case "listEvent":
		return b.RunListEvent(ctx, task, credential)
	case "listLimitRange":
		return b.RunListLimitRange(ctx, task, credential)
	case "listNamespace":
		return b.RunListNamespace(ctx, task, credential)
	case "listNode":
		return b.RunListNode(ctx, task, credential)
	case "listPersistentVolume":
		return b.RunListPersistentVolume(ctx, task, credential)
	case "listPersistentVolumeClaim":
		return b.RunListPersistentVolumeClaim(ctx, task, credential)
	case "listPod":
		return b.RunListPod(ctx, task, credential)
	case "listPodTemplate":
		return b.RunListPodTemplate(ctx, task, credential)
	case "listReplicationController":
		return b.RunListReplicationController(ctx, task, credential)
	case "listResourceQuota":
		return b.RunListResourceQuota(ctx, task, credential)
	case "listService":
		return b.RunListService(ctx, task, credential)
	case "listServiceAccount":
		return b.RunListServiceAccount(ctx, task, credential)
	case "patchConfigMap":
		return b.RunPatchConfigMap(ctx, task, credential)
	case "patchEndpoints":
		return b.RunPatchEndpoints(ctx, task, credential)
	case "patchEvent":
		return b.RunPatchEvent(ctx, task, credential)
	case "patchLimitRange":
		return b.RunPatchLimitRange(ctx, task, credential)
	case "patchNamespace":
		return b.RunPatchNamespace(ctx, task, credential)
	case "patchNode":
		return b.RunPatchNode(ctx, task, credential)
	case "patchPersistentVolume":
		return b.RunPatchPersistentVolume(ctx, task, credential)
	case "patchPersistentVolumeClaim":
		return b.RunPatchPersistentVolumeClaim(ctx, task, credential)
	case "patchPod":
		return b.RunPatchPod(ctx, task, credential)
	case "patchPodTemplate":
		return b.RunPatchPodTemplate(ctx, task, credential)
	case "patchReplicationController":
		return b.RunPatchReplicationController(ctx, task, credential)
	case "patchResourceQuota":
		return b.RunPatchResourceQuota(ctx, task, credential)
	case "patchService":
		return b.RunPatchService(ctx, task, credential)
	case "patchServiceAccount":
		return b.RunPatchServiceAccount(ctx, task, credential)
	case "updateConfigMap":
		return b.RunUpdateConfigMap(ctx, task, credential)
	case "updateEndpoints":
		return b.RunUpdateEndpoints(ctx, task, credential)
	case "updateEvent":
		return b.RunUpdateEvent(ctx, task, credential)
	case "updateLimitRange":
		return b.RunUpdateLimitRange(ctx, task, credential)
	case "updateNamespace":
		return b.RunUpdateNamespace(ctx, task, credential)
	case "updateNode":
		return b.RunUpdateNode(ctx, task, credential)
	case "updatePersistentVolume":
		return b.RunUpdatePersistentVolume(ctx, task, credential)
	case "updatePersistentVolumeClaim":
		return b.RunUpdatePersistentVolumeClaim(ctx, task, credential)
	case "updatePod":
		return b.RunUpdatePod(ctx, task, credential)
	case "updatePodTemplate":
		return b.RunUpdatePodTemplate(ctx, task, credential)
	case "updateReplicationController":
		return b.RunUpdateReplicationController(ctx, task, credential)
	case "updateResourceQuota":
		return b.RunUpdateResourceQuota(ctx, task, credential)
	case "updateService":
		return b.RunUpdateService(ctx, task, credential)
	case "updateServiceAccount":
		return b.RunUpdateServiceAccount(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *KubernetesCore) GetAction(_ string) types.Action {
	return b
}
