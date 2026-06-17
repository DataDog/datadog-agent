// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"github.com/gobwas/glob"

	k8smetadata "github.com/DataDog/datadog-agent/comp/core/tagger/k8s_metadata"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// kueueQueueTagResolver resolves the label/annotation tags of a Kueue queue on
// the cluster agent so that only the final tag strings are streamed to node
// agents. This keeps the streaming protobuf message free of arbitrary label and
// annotation maps.
//
// Only tags derived from kubernetes_resources_{labels,annotations}_as_tags are
// resolved here; the queue's intrinsic tags (kueue_local_queue,
// kueue_cluster_queue, kube_namespace) are recomputed by the tagger on the node
// side from the queue's other fields, so they are not part of the resolved tags.
type kueueQueueTagResolver struct {
	labelsAsTags      map[string]map[string]string
	annotationsAsTags map[string]map[string]string
	globLabels        map[string]map[string]glob.Glob
	globAnnotations   map[string]map[string]glob.Glob
}

func newKueueQueueTagResolver(cfg pkgconfigmodel.Reader) *kueueQueueTagResolver {
	r := &kueueQueueTagResolver{
		labelsAsTags:      map[string]map[string]string{},
		annotationsAsTags: map[string]map[string]string{},
		globLabels:        map[string]map[string]glob.Glob{},
		globAnnotations:   map[string]map[string]glob.Glob{},
	}

	if cfg == nil {
		return r
	}

	metadataAsTags := configutils.GetMetadataAsTags(cfg)

	for resource, labelsAsTags := range metadataAsTags.GetResourcesLabelsAsTags() {
		r.labelsAsTags[resource], r.globLabels[resource] = k8smetadata.InitMetadataAsTags(labelsAsTags)
	}
	for resource, annotationsAsTags := range metadataAsTags.GetResourcesAnnotationsAsTags() {
		r.annotationsAsTags[resource], r.globAnnotations[resource] = k8smetadata.InitMetadataAsTags(annotationsAsTags)
	}

	return r
}

// resolveKueueQueueTags returns the queue's label/annotation tags already
// resolved against the configuration. Each returned entry is in "name:value"
// form where a leading '+' on the name denotes a high-cardinality tag, matching
// the convention understood by taglist.AddAuto on the node side.
func (r *kueueQueueTagResolver) resolveKueueQueueTags(queue *workloadmeta.KubernetesKueueQueue) []string {
	if r == nil {
		return nil
	}

	groupResource := kueueQueueGroupResource(queue.QueueType)
	if groupResource == "" {
		return nil
	}

	tagList := taglist.NewTagList()
	for name, value := range queue.Labels {
		k8smetadata.AddMetadataAsTags(name, value, r.labelsAsTags[groupResource], r.globLabels[groupResource], tagList)
	}
	for name, value := range queue.Annotations {
		k8smetadata.AddMetadataAsTags(name, value, r.annotationsAsTags[groupResource], r.globAnnotations[groupResource], tagList)
	}

	low, _, high, _ := tagList.Compute()
	if len(low)+len(high) == 0 {
		return nil
	}

	resolved := make([]string, 0, len(low)+len(high))
	resolved = append(resolved, low...)
	for _, tag := range high {
		resolved = append(resolved, "+"+tag)
	}
	return resolved
}

func kueueQueueGroupResource(queueType workloadmeta.KueueQueueType) string {
	switch queueType {
	case workloadmeta.KueueLocalQueue:
		return kubernetes.KueueLocalQueueResourceName + "." + kubernetes.KueueGroupName
	case workloadmeta.KueueClusterQueue:
		return kubernetes.KueueClusterQueueResourceName + "." + kubernetes.KueueGroupName
	default:
		return ""
	}
}
