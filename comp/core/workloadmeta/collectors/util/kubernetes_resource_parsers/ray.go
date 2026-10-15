// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"regexp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type rayServiceParser struct {
	gvr               schema.GroupVersionResource
	annotationsFilter []*regexp.Regexp
}

// NewRayServiceParser parses only the LLM detection result and cluster names.
func NewRayServiceParser(gvr schema.GroupVersionResource, annotationsExclude ...string) (ObjectParser, error) {
	filters, err := ParseFilters(annotationsExclude)
	if err != nil {
		return nil, err
	}
	return rayServiceParser{gvr: gvr, annotationsFilter: filters}, nil
}

func (p rayServiceParser) Parse(obj interface{}) workloadmeta.Entity {
	service := obj.(*unstructured.Unstructured)
	result := &workloadmeta.RayServiceMetadata{}
	config, _, _ := unstructured.NestedString(service.Object, "spec", "serveConfigV2")
	result.IsLLM = kubernetes.RayServiceHasLLMConfig(config)
	// Also check the head and worker templates, as the backend does.
	cluster, _, _ := unstructured.NestedMap(service.Object, "spec", "rayClusterConfig")
	var templates struct {
		HeadGroupSpec struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"headGroupSpec"`
		WorkerGroupSpecs []struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"workerGroupSpecs"`
	}

	if runtime.DefaultUnstructuredConverter.FromUnstructured(cluster, &templates) == nil {
		for _, c := range templates.HeadGroupSpec.Template.Spec.Containers {
			result.IsLLM = result.IsLLM || kubernetes.IsRayLLMContainer(c.Image, c.Command, c.Args)
		}
		for _, g := range templates.WorkerGroupSpecs {
			for _, c := range g.Template.Spec.Containers {
				result.IsLLM = result.IsLLM || kubernetes.IsRayLLMContainer(c.Image, c.Command, c.Args)
			}
		}
	}
	for _, state := range []string{"activeServiceStatus", "pendingServiceStatus"} {
		if name, _, _ := unstructured.NestedString(service.Object, "status", state, "rayClusterName"); name != "" {
			result.ClusterNames = append(result.ClusterNames, name)
		}
	}
	return &workloadmeta.KubernetesMetadata{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesMetadata, ID: string(util.GenerateKubeMetadataEntityID("ray.io", "rayservices", service.GetNamespace(), service.GetName()))},
		EntityMeta: workloadmeta.EntityMeta{Name: service.GetName(), Namespace: service.GetNamespace(), UID: string(service.GetUID()), Labels: service.GetLabels(), Annotations: FilterMapStringKey(service.GetAnnotations(), p.annotationsFilter)},
		GVR:        &p.gvr, RayService: result,
	}
}
