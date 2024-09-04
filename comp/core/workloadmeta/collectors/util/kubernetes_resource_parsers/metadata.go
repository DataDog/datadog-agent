// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubemetadata"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type metadataParser struct {
	gvr               *schema.GroupVersionResource
	annotationsFilter []*regexp.Regexp
}

// NewMetadataParser initialises and returns a metadata parser
func NewMetadataParser(gvr schema.GroupVersionResource, annotationsExclude []string) (ObjectParser, error) {
	filters, err := parseFilters(annotationsExclude)
	if err != nil {
		return nil, err
	}

	return metadataParser{gvr: &gvr, annotationsFilter: filters}, nil
}

func (p metadataParser) Parse(obj interface{}) workloadmeta.Entity {
	partialObjectMetadata := obj.(*metav1.PartialObjectMetadata)
	id := kubemetadata.GenerateEntityID(p.gvr.Group, p.gvr.Resource, partialObjectMetadata.Namespace, partialObjectMetadata.Name)

	return &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(id),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        partialObjectMetadata.Name,
			Namespace:   partialObjectMetadata.Namespace,
			Labels:      partialObjectMetadata.Labels,
			Annotations: filterMapStringKey(partialObjectMetadata.Annotations, p.annotationsFilter),
		},
		GVR: p.gvr,
	}
}
