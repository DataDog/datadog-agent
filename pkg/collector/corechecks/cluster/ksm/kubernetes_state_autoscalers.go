// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// deploymentKey and statefulSetKey are the KSM label keys naming the
	// workload of kube_deployment_* and kube_statefulset_* series.
	deploymentKey  = "deployment"
	statefulSetKey = "statefulset"

	autoscalerKindTagPrefix = tags.KubeAutoscalerKind + ":"
)

// argoRolloutPodLabels stands in for the labels of a pod managed by Argo
// Rollouts, for kubernetes.ResolveWorkloadTarget. Read-only.
var argoRolloutPodLabels = map[string]string{kubernetes.ArgoRolloutLabelKey: "true"}

// seriesWorkloadTarget returns the workload a series belongs to: the owner of a
// pod or container series (resolved like the workloadmeta collectors do, so
// both sides agree on a pod's workload), or the Deployment or StatefulSet a
// kube_deployment_* or kube_statefulset_* series describes.
func seriesWorkloadTarget(labels map[string]string, namespace, ownerKind, ownerName string, isArgoRollout bool) (kubernetes.WorkloadTarget, bool) {
	if namespace == "" {
		return kubernetes.WorkloadTarget{}, false
	}

	if ownerKind != "" && ownerName != "" {
		var podLabels map[string]string
		if isArgoRollout {
			podLabels = argoRolloutPodLabels
		}
		return kubernetes.ResolveWorkloadTarget(namespace, ownerKind, ownerName, podLabels)
	}

	if name := labels[deploymentKey]; name != "" {
		return kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: namespace, Name: name}, true
	}
	if name := labels[statefulSetKey]; name != "" {
		return kubernetes.WorkloadTarget{Kind: kubernetes.StatefulSetKind, Namespace: namespace, Name: name}, true
	}
	return kubernetes.WorkloadTarget{}, false
}

// autoscalerTags returns the kube_autoscaler_kind tags of a workload, as the
// Cluster Agent's tagger publishes them on the workload's entity. Only those
// tags are kept: the entity may also carry labels or annotations as tags
// configured for workloads, which are not meant for KSM series.
//
// Results are cached for the current check run, so the tagger is queried once
// per workload per run rather than once per series.
func (k *KSMCheck) autoscalerTags(target kubernetes.WorkloadTarget) []string {
	if cached, found := k.autoscalerTagsCache[target]; found {
		return cached
	}

	var result []string
	if entityID, ok := workloadTaggerEntityID(target); ok {
		entityTags, err := k.tagger.Tag(entityID, types.LowCardinality)
		if err != nil {
			if k.autoscalerTagsErrorLogLimit.ShouldLog() {
				log.Debugf("failed to get autoscaler tags for %s %s/%s from tagger: %v", target.Kind, target.Namespace, target.Name, err)
			}
		}
		for _, tag := range entityTags {
			if strings.HasPrefix(tag, autoscalerKindTagPrefix) {
				result = append(result, tag)
			}
		}
	}

	if k.autoscalerTagsCache == nil {
		k.autoscalerTagsCache = make(map[kubernetes.WorkloadTarget][]string)
	}
	k.autoscalerTagsCache[target] = result
	return result
}

// workloadTaggerEntityID returns the tagger entity carrying a workload's
// autoscaler kinds: a KubernetesDeployment for Deployments, the generic
// KubernetesMetadata entity for StatefulSets and Argo Rollouts.
func workloadTaggerEntityID(target kubernetes.WorkloadTarget) (types.EntityID, bool) {
	if target.Kind == kubernetes.DeploymentKind {
		return types.NewEntityID(types.KubernetesDeployment, target.Namespace+"/"+target.Name), true
	}
	metadata, ok := util.WorkloadMetadataEntity(target)
	if !ok {
		return types.EntityID{}, false
	}
	return types.NewEntityID(types.KubernetesMetadata, metadata.ID), true
}
