// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/otelinstrumentation"
)

// NamespaceAnnotationGetter reads namespace annotations from workloadmeta, which the
// admission path can query without an API call. It implements
// otelinstrumentation.NamespaceAnnotationGetter.
type NamespaceAnnotationGetter struct {
	wmeta workloadmeta.Component
}

// NewNamespaceAnnotationGetter returns a getter reading namespace metadata from wmeta.
func NewNamespaceAnnotationGetter(wmeta workloadmeta.Component) *NamespaceAnnotationGetter {
	return &NamespaceAnnotationGetter{wmeta: wmeta}
}

// NamespaceAnnotations returns the annotations of the named namespace.
//
// The boolean reports whether the namespace could be read at all, not whether it holds
// any annotation: a namespace missing from the store must not look like a namespace that
// genuinely carries none, because the two lead the OpenTelemetry annotation contract to
// different answers. A namespace that exists with no annotations therefore returns a nil
// map and true.
func (g *NamespaceAnnotationGetter) NamespaceAnnotations(namespace string) (map[string]string, bool) {
	if g == nil || g.wmeta == nil {
		return nil, false
	}

	id := util.GenerateKubeMetadataEntityID("", "namespaces", "", namespace)
	ns, err := g.wmeta.GetKubernetesMetadata(id)
	if err != nil {
		return nil, false
	}

	return ns.EntityMeta.Annotations, true
}

// Compile-time check that the getter satisfies the interface the resolver expects.
var _ otelinstrumentation.NamespaceAnnotationGetter = (*NamespaceAnnotationGetter)(nil)
