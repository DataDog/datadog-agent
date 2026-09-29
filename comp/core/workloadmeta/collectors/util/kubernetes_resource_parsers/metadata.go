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

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type KindMapper map[string]metav1.APIResource

type metadataParser struct {
	gvr               *schema.GroupVersionResource
	annotationsFilter []*regexp.Regexp
	kindMapper        KindMapper
}

// NewMetadataParser initialises and returns a metadata parser
func NewMetadataParser(gvr schema.GroupVersionResource, annotationsExclude []string, kindMapper KindMapper) (ObjectParser, error) {
	filters, err := ParseFilters(annotationsExclude)
	if err != nil {
		return nil, err
	}

	return metadataParser{gvr: &gvr, annotationsFilter: filters, kindMapper: kindMapper}, nil
}

func (p metadataParser) Parse(obj interface{}) workloadmeta.Entity {
	partialObjectMetadata := obj.(*metav1.PartialObjectMetadata)
	id := util.GenerateKubeMetadataEntityID(p.gvr.Group, p.gvr.Resource, partialObjectMetadata.Namespace, partialObjectMetadata.Name)
	var owners []workloadmeta.EntityID

	log.Debugf("Parsing %s %s/%s and found %d owner references",
		p.gvr.Resource,
		partialObjectMetadata.Namespace,
		partialObjectMetadata.Name,
		len(partialObjectMetadata.GetOwnerReferences()),
	)

	for _, ownerRef := range partialObjectMetadata.GetOwnerReferences() {

		var ownerKind workloadmeta.Kind
		var ownerRefID string
		switch ownerRef.Kind {
		case "Deployment":
			ownerKind = workloadmeta.KindKubernetesDeployment
			ownerRefID = partialObjectMetadata.Namespace + "/" + ownerRef.Name
		default:
			ownerKind = workloadmeta.KindKubernetesMetadata
			ownerRefID = string(util.GenerateKubeMetadataEntityID(
				p.kindMapper[ownerRef.Kind].Group,
				p.kindMapper[ownerRef.Kind].Name,
				partialObjectMetadata.Namespace,
				ownerRef.Name),
			)
		}

		newOwner := workloadmeta.EntityID{
			Kind: ownerKind,
			ID:   ownerRefID,
		}

		log.Debugf("Owner = GVR: %s/%s/%s, Kind: %s, ID: %s",
			p.kindMapper[ownerRef.Kind].Group,
			p.kindMapper[ownerRef.Kind].Version,
			p.kindMapper[ownerRef.Kind].Name,
			newOwner.Kind,
			newOwner.ID,
		)

		owners = append(owners, newOwner)
	}

	return &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(id),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:            partialObjectMetadata.Name,
			Namespace:       partialObjectMetadata.Namespace,
			Labels:          partialObjectMetadata.Labels,
			Annotations:     FilterMapStringKey(partialObjectMetadata.Annotations, p.annotationsFilter),
			OwnerReferences: owners,
			UID:             string(partialObjectMetadata.UID),
		},
		GVR: p.gvr,
	}
}
