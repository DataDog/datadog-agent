// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package processors

import (
	"fmt"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	"github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	kubetypes "github.com/DataDog/datadog-agent/third_party/kubernetes/pkg/kubelet/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// K8sPodHandlers implements the Handlers interface for Kubernetes Pods.
type K8sPodHandlers struct{}

// AfterMarshalling is a handler called after resource marshalling.
func (h *K8sPodHandlers) AfterMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.Pod)
	m.Yaml = yaml
	return
}

// BeforeCacheCheck is a handler called before cache lookup.
func (h *K8sPodHandlers) BeforeCacheCheck(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*corev1.Pod)
	m := resourceModel.(*model.Pod)

	// static pods "uid" are actually not unique across nodes.
	// we differ from the k8 uuid format in purpose to differentiate those static pods.
	if kubetypes.IsStaticPod(r) {
		newUID := transformers.GenerateUniqueK8sStaticPodHash(ctx.HostName, r.Name, r.Namespace, ctx.Cfg.KubeClusterName)
		// modify it in the original pod for the YAML and in our model
		r.UID = types.UID(newUID)
		m.Metadata.Uid = newUID
	}

	// insert tagger tags
	tags, err := tagger.Tag(kubelet.PodUIDToTaggerEntityName(string(r.UID)), collectors.HighCardinality)
	if err != nil {
		log.Debugf("Could not retrieve tags for pod: %s", err)
		skip = true
		return
	}

	// additional tags
	m.Tags = append(tags, fmt.Sprintf("pod_status:%s", strings.ToLower(m.Status)))

	// tags that should be on the tagger
	if len(tags) == 0 {
		// Tags which should be on the tagger
		for _, volume := range r.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName != "" {
				tag := fmt.Sprintf("%s:%s", kubernetes.PersistentVolumeClaimTagName, strings.ToLower(volume.PersistentVolumeClaim.ClaimName))
				m.Tags = append(m.Tags, tag)
			}
		}
	}

	// Custom resource version to work around kubelet issues.
	if err := transformers.FillK8sPodResourceVersion(m); err != nil {
		log.Warnf("Failed to compute pod resource version: %s", err)
		skip = true
		return
	}

	return
}

// BeforeMarshalling is a handler called before resource marshalling.
func (h *K8sPodHandlers) BeforeMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *K8sPodHandlers) BuildMessageBody(ctx *ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.Pod, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.Pod))
	}

	return &model.CollectorPod{
		ClusterName: ctx.Cfg.KubeClusterName,
		ClusterId:   ctx.ClusterID,
		GroupId:     ctx.MsgGroupID,
		GroupSize:   int32(groupSize),
		Pods:        models,
		Tags:        ctx.Cfg.ExtraTags,
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *K8sPodHandlers) ExtractResource(ctx *ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*corev1.Pod)
	return transformers.ExtractK8sPod(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *K8sPodHandlers) ResourceList(ctx *ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*corev1.Pod)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *K8sPodHandlers) ResourceUID(ctx *ProcessorContext, resource, resourceModel interface{}) types.UID {
	return resource.(*corev1.Pod).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *K8sPodHandlers) ResourceVersion(ctx *ProcessorContext, resource, resourceModel interface{}) string {
	return resourceModel.(*model.Pod).Metadata.ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *K8sPodHandlers) ScrubBeforeExtraction(ctx *ProcessorContext, resource interface{}) {
	r := resource.(*corev1.Pod)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (h *K8sPodHandlers) ScrubBeforeMarshalling(ctx *ProcessorContext, resource interface{}) {
	r := resource.(*corev1.Pod)
	if ctx.Cfg.IsScrubbingEnabled {
		for c := 0; c < len(r.Spec.Containers); c++ {
			redact.ScrubContainer(&r.Spec.Containers[c], ctx.Cfg.Scrubber)
		}
		for c := 0; c < len(r.Spec.InitContainers); c++ {
			redact.ScrubContainer(&r.Spec.InitContainers[c], ctx.Cfg.Scrubber)
		}
	}

}
