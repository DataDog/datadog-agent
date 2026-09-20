// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// sourceKindGVRs maps supported v1 source kinds to their resource. Kinds not
// listed here are rejected at reconcile time with a rule error.
var sourceKindGVRs = map[string]schema.GroupVersionResource{
	"pod":        {Group: "", Version: "v1", Resource: "pods"},
	"node":       {Group: "", Version: "v1", Resource: "nodes"},
	"namespace":  {Group: "", Version: "v1", Resource: "namespaces"},
	"lease":      {Group: "coordination.k8s.io", Version: "v1", Resource: "leases"},
	"deployment": {Group: "apps", Version: "v1", Resource: "deployments"},
	"service":    {Group: "", Version: "v1", Resource: "services"},
}

// normalizeKind lowercases and trims a kind so "Lease" and "lease" resolve
// identically.
func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

// resolveSourceGVR returns the resource for a source reference. A source with
// the entity's own kind and no name refers to the entity itself.
func resolveSourceGVR(src *SourceRef) (schema.GroupVersionResource, error) {
	if src == nil {
		return schema.GroupVersionResource{}, fmt.Errorf("nil source reference")
	}
	kind := normalizeKind(src.Kind)
	if kind == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("source kind must not be empty")
	}
	gvr, ok := sourceKindGVRs[kind]
	if !ok {
		return schema.GroupVersionResource{}, fmt.Errorf("unsupported source kind %q in v1", src.Kind)
	}
	if src.APIVersion != "" && src.APIVersion != gvr.Group+"/"+gvr.Version {
		return schema.GroupVersionResource{}, fmt.Errorf("source apiVersion %q does not match kind %q (expected %s)", src.APIVersion, src.Kind, gvr.Group+"/"+gvr.Version)
	}
	return gvr, nil
}

// sourceIsSelf reports whether the source reference resolves to the entity
// itself: no source at all, or the entity's kind without a name expression.
func sourceIsSelf(rule *TagRule) bool {
	if rule.Spec.Source == nil {
		return true
	}
	if normalizeKind(rule.Spec.Source.Kind) != normalizeKind(string(rule.Spec.Entity)) {
		return false
	}
	return rule.Spec.Source.Name == ""
}

// resolvedSource identifies the concrete object a rule's source resolves to
// for one entity.
type resolvedSource struct {
	gvr       schema.GroupVersionResource
	namespace string
	name      string
}

// resolveSource computes the source object coordinates for an entity: the
// name and namespace CEL expressions are evaluated against the entity, with
// empty name defaulting to the entity's own name and empty namespace to the
// entity's namespace (namespaced sources).
func resolveSource(rule *TagRule, entity *unstructured.Unstructured, evaluator *Evaluator) (resolvedSource, bool, error) {
	if sourceIsSelf(rule) {
		// Source is the entity; the caller passes the entity again.
		return resolvedSource{}, false, nil
	}

	gvr, err := resolveSourceGVR(rule.Spec.Source)
	if err != nil {
		return resolvedSource{}, false, err
	}

	entityMap := entity.Object
	name := rule.Spec.Source.Name
	if name == "" {
		name = entity.GetName()
	} else {
		resolved, err := evaluator.EvalString(name, entityMap, entityMap)
		if err != nil {
			return resolvedSource{}, false, fmt.Errorf("resolving source name: %w", err)
		}
		if resolved == "" {
			return resolvedSource{}, false, fmt.Errorf("source name expression %q resolved to an empty name", name)
		}
		name = resolved
	}

	namespace := rule.Spec.Source.Namespace
	if namespace == "" {
		namespace = entity.GetNamespace()
	} else {
		resolved, err := evaluator.EvalString(namespace, entityMap, entityMap)
		if err != nil {
			return resolvedSource{}, false, fmt.Errorf("resolving source namespace: %w", err)
		}
		namespace = resolved
	}
	// Cluster-scoped sources ignore the namespace.
	if !isNamespacedGVR(gvr) {
		namespace = ""
	}

	return resolvedSource{gvr: gvr, namespace: namespace, name: name}, true, nil
}

// isNamespacedGVR reports whether the resource is namespaced. The v1 source
// table only contains Lease and Deployment as namespaced kinds.
func isNamespacedGVR(gvr schema.GroupVersionResource) bool {
	switch gvr.Resource {
	case "nodes", "namespaces":
		return false
	default:
		return true
	}
}
