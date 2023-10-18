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
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var (
	volumeTagKeysToExclude = []string{"persistentvolumeclaim", "pod_phase"}
)

// PodUtils is used to cache computed pod metadata during check execution, which would otherwise be too
// computationally heavy to do in place.
type PodUtils struct {
	podTagsByPVC map[string][]string
}

// NewPodUtils creates a new instance of PodUtils
func NewPodUtils() *PodUtils {
	return &PodUtils{podTagsByPVC: map[string][]string{}}
}

// Reset sets the PodUtils instance back to a default state. It should be called at the end of a check run to prevent
// stale data from impacting overall memory usage.
func (p *PodUtils) Reset() {
	p.podTagsByPVC = map[string][]string{}
}

// ComputePodTagsByPVC stores the tags for a given pod in a global caching layer, indexed by pod namespace and persistent
// volume name.
func (p *PodUtils) ComputePodTagsByPVC(pod *kubelet.Pod) {
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
				p.podTagsByPVC[fmt.Sprintf("%s/%s", pod.Metadata.Namespace, pvcName)] = filteredTags
			}
		}

		// get standalone PVC associated to potential EVC
		// when a generic ephemeral volume is created, an associated pvc named <pod_name>-<volume_name>
		// is created (https://docs.openshift.com/container-platform/4.11/storage/generic-ephemeral-vols.html).
		if v.Ephemeral != nil {
			ephemeral := v.Ephemeral.VolumeClaimTemplate
			volumeName := v.Name
			if ephemeral != nil && volumeName != "" {
				p.podTagsByPVC[fmt.Sprintf("%s/%s-%s", pod.Metadata.Namespace, pod.Metadata.Name, volumeName)] = filteredTags
			}
		}
	}
}

// GetPodTagsByPVC returns the computed pod tags for a PVC with a given name in a given namespace.
func (p *PodUtils) GetPodTagsByPVC(namespace, pvcName string) []string {
	return p.podTagsByPVC[fmt.Sprintf("%s/%s", namespace, pvcName)]
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
	for _, c := range pod.GetAllContainers() {
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
