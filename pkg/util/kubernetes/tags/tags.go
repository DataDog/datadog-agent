// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tags

import (
	"errors"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	tagsCacheTTL       = 1 * time.Hour
	tagsCacheKeyPrefix = "metaTags-"
)

// UnstructuredTotags extracts tags for an unstructured Kubernetes objects.
// Use MetaToTags for structured objects.
// Results are cached in-memory for 1 hour.
func UnstructuredTotags(obj *unstructured.Unstructured) ([]string, error) {
	if obj == nil {
		return nil, errors.New("nil object")
	}

	return MetaToTags(obj.GetKind(), obj)
}

// MetaToTags extracts tags for structured Kubernetes objects based on kind and metadata.
// Use UnstructuredTotags for unstructured objects.
// Kind string (e.g Deployment, Pod) needs to be provided manually by the caller, see https://github.com/kubernetes/client-go/issues/541
// Results are cached in-memory for 1 hour.
func MetaToTags(kind string, meta metav1.Object) ([]string, error) {
	objUID := string(meta.GetUID())
	if objUID == "" {
		return nil, errors.New("empty object UID")
	}

	tagsCacheKey := tagsCacheKeyPrefix + objUID
	if tagsCache, found := cache.Cache.Get(tagsCacheKey); found {
		tags, ok := tagsCache.([]string)
		if !ok {
			return nil, errors.New("couldn't cast tags list from cache")
		}

		return tags, nil
	}

	builder := newTagListBuilder()
	builder.addNotEmpty(kubernetes.NamespaceTagName, meta.GetNamespace())
	builder.addNotEmpty(kubernetes.ResourceNameTagName, meta.GetName())
	builder.addNotEmpty(kubernetes.ResourceKindTagName, strings.ToLower(kind))
	builder.addNotEmpty(kubernetes.KindToTagName[kind], meta.GetName())

	for _, owner := range meta.GetOwnerReferences() {
		builder.addNotEmpty(kubernetes.OwnerRefNameTagName, owner.Name)
		builder.addNotEmpty(kubernetes.OwnerRefKindTagName, strings.ToLower(owner.Kind))
		builder.addNotEmpty(kubernetes.KindToTagName[owner.Kind], owner.Name)
	}

	tags := builder.tags()
	cache.Cache.Set(tagsCacheKey, tags, tagsCacheTTL)

	return tags, nil
}
