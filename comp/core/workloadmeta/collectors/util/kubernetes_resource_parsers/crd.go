// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type crdParser struct{}

// NewCRDParser initialises and returns a CRD parser
func NewCRDParser() ObjectParser {
	return crdParser{}
}

func (p crdParser) Parse(obj interface{}) workloadmeta.Entity {
	crd := obj.(*apiextensionsv1.CustomResourceDefinition)

	// Use the first version as the primary version
	// CRDs can have multiple versions, but we'll track the stored version
	var version string
	for _, v := range crd.Spec.Versions {
		if v.Storage {
			version = v.Name
			break
		}
	}
	// Fallback to first version if no storage version is found
	if version == "" && len(crd.Spec.Versions) > 0 {
		version = crd.Spec.Versions[0].Name
	}

	return &workloadmeta.CRD{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindCRD,
			ID:   crd.Name, // CRD names are unique cluster-wide
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        crd.Name,
			Labels:      crd.Labels,
			Namespace:   crd.Namespace,
			Annotations: crd.Annotations,
			UID:         string(crd.UID),
		},
		Group:   crd.Spec.Group,
		Kind:    crd.Spec.Names.Kind,
		Version: version,
	}
}
