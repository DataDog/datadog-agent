// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package common

import (
	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// GetContainerId returns the container ID from the workloadmeta.Store for a given set of metric labels.
// It should only be called on a container-scoped metric. It returns an empty string if the container could not be
// found, or if the container should be filtered out.
func GetContainerId(store workloadmeta.Store, metric model.Metric, filter *containers.Filter) string {
	namespace := string(metric["namespace"])
	podUid := string(metric["pod_uid"])
	// k8s >= 1.16
	containerName := string(metric["container"])
	podName := string(metric["pod"])
	// k8s < 1.16
	if containerName == "" {
		containerName = string(metric["container_name"])
	}
	if podName == "" {
		podName = string(metric["pod_name"])
	}

	pod, err := store.GetKubernetesPod(podUid)
	if err != nil {
		pod, err = store.GetKubernetesPodByName(podName, namespace)
		if err != nil {
			log.Debugf("pod not found for id:%s, name:%s, namespace:%s", podUid, podName, namespace)
			return ""
		}
	}

	var container *workloadmeta.OrchestratorContainer
	for _, c := range pod.Containers {
		if c.Name == containerName {
			container = &c
			break
		}
	}

	if container == nil {
		log.Debugf("container %s not found for pod with name %s", containerName, pod.Name)
		return ""
	}

	if filter.IsExcluded(pod.EntityMeta.Annotations, container.Name, container.Image.Name, pod.Namespace) {
		return ""
	}

	cId := containers.BuildTaggerEntityName(container.ID)

	return cId
}
