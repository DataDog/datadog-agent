// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package common

import (
	"errors"
	"fmt"
	"strings"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

var (
	volumeTagKeysToExclude = []string{tags.KubePersistentVolumeClaim, tags.PodPhase}

	// ErrContainerExcluded is an error representing the exclusion of a container from metric collection
	ErrContainerExcluded = errors.New("container is excluded")
	// ErrContainerNotFound is an error representing the absence of a container
	ErrContainerNotFound = errors.New("container not found")
	// ErrPodNotFound is an error representing the absence of a pod
	ErrPodNotFound = errors.New("parent pod not found")
)

type podMetadata struct {
	isHostNetworked bool
	isStaticPending bool
}

// PodUtils is used to cache computed pod metadata during check execution, which would otherwise be too
// computationally heavy to do in place, or would only be used by this check so it does not make sense to
// store in the workloadmeta store.
type PodUtils struct {
	podTagsByPVC map[string][]string
	podMetadata  map[string]*podMetadata
	tagger       tagger.Component
}

// NewPodUtils creates a new instance of PodUtils
func NewPodUtils(tagger tagger.Component) *PodUtils {
	return &PodUtils{
		podTagsByPVC: map[string][]string{},
		podMetadata:  map[string]*podMetadata{},
		tagger:       tagger,
	}
}

// Reset sets the PodUtils instance back to a default state. It should be called at the end of a check run to prevent
// stale data from impacting overall memory usage.
func (p *PodUtils) Reset() {
	p.podTagsByPVC = map[string][]string{}
	p.podMetadata = map[string]*podMetadata{}
}

// PopulateForPod generates the PodUtils entries for a given pod.
func (p *PodUtils) PopulateForPod(pod *workloadmeta.KubernetesPod) {
	if pod == nil {
		return
	}

	// populate the pod tags by PVC name
	p.computePodTagsByPVC(pod)

	// populate the pod metadata
	isHostNetworked := pod.HostNetwork
	isStaticPending := pod.Annotations != nil &&
		pod.Annotations["kubernetes.io/config.source"] != "api" &&
		pod.Phase == "Pending" && len(pod.ContainerStatuses) == 0
	p.podMetadata[pod.ID] = &podMetadata{
		isHostNetworked: isHostNetworked,
		isStaticPending: isStaticPending,
	}
}

// computePodTagsByPVC stores the tags for a given pod in a global caching layer, indexed by pod namespace and persistent
// volume name.
func (p *PodUtils) computePodTagsByPVC(pod *workloadmeta.KubernetesPod) {
	podUID := types.NewEntityID(types.KubernetesPodUID, pod.ID)
	tags, _ := p.tagger.Tag(podUID, types.OrchestratorCardinality)
	if len(tags) == 0 {
		return
	}

	filteredTags := make([]string, 0, len(tags))
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

	for _, v := range pod.Volumes {
		if v.PersistentVolumeClaim != nil {
			pvcName := v.PersistentVolumeClaim.ClaimName
			if pvcName != "" {
				p.podTagsByPVC[pod.Namespace+"/"+pvcName] = filteredTags
			}
		}

		// get standalone PVC associated to potential EVC
		// when a generic ephemeral volume is created, an associated pvc named <pod_name>-<volume_name>
		// is created (https://docs.openshift.com/container-platform/4.11/storage/generic-ephemeral-vols.html).
		if v.Ephemeral != nil {
			volumeName := v.Name
			if volumeName != "" {
				p.podTagsByPVC[fmt.Sprintf("%s/%s-%s", pod.Namespace, pod.Name, volumeName)] = filteredTags
			}
		}
	}
}

// GetPodTagsByPVC returns the computed pod tags for a PVC with a given name in a given namespace.
func (p *PodUtils) GetPodTagsByPVC(namespace, pvcName string) []string {
	return p.podTagsByPVC[namespace+"/"+pvcName]
}

// IsStaticPendingPod returns whether the pod with the given UID is a static pending pod or not, or returns false
// if the pod cannot be found. This is due to a bug where the kubelet pod list is not updated for static pods in
// k8s <1.15. This has been fixed here: https://github.com/kubernetes/kubernetes/pull/77661
func (p *PodUtils) IsStaticPendingPod(podUID string) bool {
	if meta, ok := p.podMetadata[podUID]; ok {
		return meta.isStaticPending
	}
	return false
}

// IsHostNetworkedPod returns whether the pod is on a host network or not. It returns false if the pod cannot be found.
func (p *PodUtils) IsHostNetworkedPod(podUID string) bool {
	if meta, ok := p.podMetadata[podUID]; ok {
		return meta.isHostNetworked
	}
	return false
}

// GetContainerID returns the container ID from the workloadmeta.Component for a given set of metric labels.
// It should only be called on a container-scoped metric. It returns an empty string if the container could not be
// found, or if the container should be filtered out.
func GetContainerID(store workloadmeta.Component, metric prometheus.Metric, containerFilter workloadfilter.FilterBundle) (string, error) {
	namespace := metric["namespace"]
	podUID := metric["pod_uid"]
	// k8s >= 1.16
	containerName := metric["container"]
	podName := metric["pod"]
	// k8s < 1.16
	if containerName == "" {
		containerName = metric["container_name"]
	}
	if podName == "" {
		podName = metric["pod_name"]
	}

	pod, err := store.GetKubernetesPod(podUID)
	if err != nil {
		pod, err = store.GetKubernetesPodByName(podName, namespace)
		if err != nil {
			log.Debugf("pod not found for id:%s, name:%s, namespace:%s", podUID, podName, namespace)
			return "", ErrPodNotFound
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
		return "", ErrContainerNotFound
	}

	filterableContainer := workloadmetafilter.CreateContainerFromOrch(container, workloadmetafilter.CreatePod(pod))
	if containerFilter.IsExcluded(filterableContainer) {
		return "", ErrContainerExcluded
	}

	return container.ID, nil
}

// AppendKubeStaticCPUsTag accepts a list of tags and returns
// a list of tags with the proper kube_requested_cpu_management tag appended
func AppendKubeStaticCPUsTag(w workloadmeta.Component, qos string, containerID types.EntityID, tagList []string) []string {
	wmetaKubelet, _ := w.GetKubelet()
	if wmetaKubelet != nil {
		cpuManagerPolicy := wmetaKubelet.ConfigDocument.KubeletConfig.CPUManagerPolicy

		wmetaContainer, err := w.GetContainer(containerID.GetID())
		if err != nil {
			log.Errorf("error getting container with ID '%s': %v", containerID.GetID(), err)
			return tagList
		}

		requestedWholeCores := false
		if wmetaContainer.Resources.RequestedWholeCores != nil {
			requestedWholeCores = *wmetaContainer.Resources.RequestedWholeCores
		}

		if qos == "Guaranteed" &&
			requestedWholeCores &&
			cpuManagerPolicy == "static" {
			tagList = utils.ConcatenateStringTags(tagList, tags.KubeStaticCPUsTag+":true")
		} else {
			tagList = utils.ConcatenateStringTags(tagList, tags.KubeStaticCPUsTag+":false")
		}
	}

	return tagList
}
