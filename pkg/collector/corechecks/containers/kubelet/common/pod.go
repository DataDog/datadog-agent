// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package common

import (
	"fmt"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var (
	volumeTagKeysToExclude = []string{"persistentvolumeclaim", "pod_phase"}
)

// CachePodTagsByPVC stores the tags for a given pod in a global caching layer, indexed by pod namespace and persistent
// volume name.
func CachePodTagsByPVC(pod *kubelet.Pod) {
	// TODO
	podUID := kubelet.PodUIDToTaggerEntityName(pod.Metadata.UID)
	tags, _ := tagger.Tag(podUID, collectors.OrchestratorCardinality)
	if len(tags) == 0 {
		return
	}

	var filteredTags []string
	for t := range tags {
		omitTag := false
		for i := range volumeTagKeysToExclude {
			if strings.HasPrefix(tags[t], volumeTagKeysToExclude[i]+":") {
				omitTag = true
				break
			}
		}
		if !omitTag {
			filteredTags = append(filteredTags, tags[t])
		}
	}

	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			pvcName := v.PersistentVolumeClaim.ClaimName
			if pvcName != "" {
				// TODO nil checks
				// TODO cache key prefix
				cache.Cache.Set(fmt.Sprintf("check/kubelet/pvc/%s/%s", pod.Metadata.Namespace, pvcName), filteredTags, 0)
			}
		}

		// get standalone PVC associated to potential EVC
		// when a generic ephemeral volume is created, an associated pvc named <pod_name>-<volume_name>
		// is created (https://docs.openshift.com/container-platform/4.11/storage/generic-ephemeral-vols.html).
		if v.Ephemeral != nil {
			ephemeral := v.Ephemeral.VolumeClaimTemplate
			volumeName := v.Name
			if ephemeral != nil && volumeName != "" {
				cache.Cache.Set(fmt.Sprintf("check/kubelet/pvc/%s/%s-%s", pod.Metadata.Namespace, pod.Metadata.Name, volumeName), filteredTags, 0)
			}
		}
	}
}

// GetContainerID returns the container ID from the workloadmeta.Store for a given set of metric labels.
// It should only be called on a container-scoped metric. It returns an empty string if the container could not be
// found, or if the container should be filtered out.
func GetContainerID(store workloadmeta.Store, metric model.Metric, filter *containers.Filter) string {
	namespace := string(metric["namespace"])
	podUID := string(metric["pod_uid"])
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

	pod, err := store.GetKubernetesPod(podUID)
	if err != nil {
		pod, err = store.GetKubernetesPodByName(podName, namespace)
		if err != nil {
			log.Debugf("pod not found for id:%s, name:%s, namespace:%s", podUID, podName, namespace)
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

	cID := containers.BuildTaggerEntityName(container.ID)

	return cID
}
