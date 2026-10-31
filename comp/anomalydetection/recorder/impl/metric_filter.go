// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// MetricFilterRequires lists the optional dependencies used to resolve AAD metric
// exclusion annotations. When either dependency is unavailable, no filter is
// created and the recorder records all metrics.
type MetricFilterRequires struct {
	WMeta       workloadmeta.Component   `optional:"true"`
	FilterStore workloadfilter.Component `optional:"true"`
}

// NewMetricFilter creates the AAD metric exclusion filter. It returns nil when
// either dependency is absent so that missing metadata remains best-effort.
func NewMetricFilter(req MetricFilterRequires) recorder.MetricFilter {
	if req.WMeta == nil || req.FilterStore == nil {
		return nil
	}
	return &metricFilter{
		wmeta:        req.WMeta,
		filterBundle: req.FilterStore.GetContainerAutodiscoveryFilters(workloadfilter.MetricsFilter),
	}
}

type metricFilter struct {
	wmeta        workloadmeta.Component
	filterBundle workloadfilter.FilterBundle
}

func (f *metricFilter) IsExcluded(sample observer.MetricView) bool {
	if f == nil || f.filterBundle == nil {
		return false
	}

	containerID, podName, containerName, namespace := metricIdentityTags(sample.GetTags())
	if containerID == "" && (podName == "" || containerName == "") {
		return false
	}

	if containerID != "" {
		container, err := f.wmeta.GetContainer(containerID)
		if err == nil && container != nil {
			return f.isExcludedContainer(container)
		}
	}

	if podName == "" || containerName == "" {
		return false
	}

	pod, err := f.wmeta.GetKubernetesPodByName(podName, namespace)
	if err != nil || pod == nil {
		return false
	}
	container := podContainer(pod, containerName)
	if container == nil {
		return false
	}
	return f.isExcludedContainer(container)
}

func (f *metricFilter) isExcludedContainer(container *workloadmeta.Container) bool {
	pod, err := f.wmeta.GetKubernetesPodForContainer(container.EntityID.ID)
	if err != nil || pod == nil {
		return false
	}
	return f.filterBundle.IsExcluded(workloadmetafilter.CreateContainer(container, workloadmetafilter.CreatePod(pod)))
}

func metricIdentityTags(tags tagset.CompositeTags) (containerID, podName, containerName, namespace string) {
	tags.ForEach(func(tag string) {
		key, value, found := strings.Cut(tag, ":")
		if !found {
			return
		}
		switch key {
		case "container_id":
			if containerID == "" {
				containerID = value
			}
		case "kube_container_name":
			if containerName == "" {
				containerName = value
			}
		case "kube_pod_name":
			if podName == "" {
				podName = value
			}
		case "pod_name":
			if podName == "" {
				podName = value
			}
		case "kube_namespace":
			if namespace == "" {
				namespace = value
			}
		}
	})
	return containerID, podName, containerName, namespace
}

func podContainer(pod *workloadmeta.KubernetesPod, containerName string) *workloadmeta.Container {
	for _, status := range pod.ContainerStatuses {
		if status.Name == containerName && status.ContainerID != "" {
			return &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: status.ContainerID},
				Image:    workloadmeta.ContainerImage{Name: status.Image},
				Runtime:  workloadmeta.ContainerRuntimeContainerd,
			}
		}
	}

	for _, container := range pod.Containers {
		if container.Name == containerName {
			return &workloadmeta.Container{
				EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: container.ID},
				EntityMeta: workloadmeta.EntityMeta{Name: container.Name},
				Image:      container.Image,
				Runtime:    workloadmeta.ContainerRuntimeContainerd,
			}
		}
	}

	return nil
}
